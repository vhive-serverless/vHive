// MIT License
//
// Copyright (c) 2020 Dmitrii Ustiugov, Plamen Petrov and EASE lab
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package ctriface

import (
	"context"
	"github.com/ease-lab/vhive/devmapper"
	"github.com/ease-lab/vhive/snapshotting"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/firecracker-microvm/firecracker-containerd/proto" // note: from the original repo
	"github.com/firecracker-microvm/firecracker-containerd/runtime/firecrackeroci"
	"github.com/pkg/errors"

	_ "google.golang.org/grpc/codes"  //tmp
	_ "google.golang.org/grpc/status" //tmp

	"github.com/ease-lab/vhive/memory/manager"
	"github.com/ease-lab/vhive/metrics"
	"github.com/ease-lab/vhive/misc"
	"github.com/go-multierror/multierror"

	_ "github.com/davecgh/go-spew/spew" //tmp
)

const (
	TestImageName = "ghcr.io/ease-lab/helloworld:var_workload"
)

// StartVM Boots a VM if it does not exist
func (o *Orchestrator) StartVM(ctx context.Context, vmID, imageName string, memSizeMib, vCPUCount uint32, trackDirtyPages bool) (_ *StartVMResponse, _ *metrics.Metric, retErr error) {
	var (
		startVMMetric *metrics.Metric = metrics.NewMetric()
		tStart        time.Time
	)

	logger := log.WithFields(log.Fields{"vmID": vmID, "image": imageName})
	logger.Debug("StartVM: Received StartVM")

	// 1. Allocate VM metadata & create vm network
	vm, err := o.vmPool.Allocate(vmID)
	if err != nil {
		logger.Error("failed to allocate VM in VM pool")
		return nil, nil, err
	}

	// Set VM vCPU and Memory
	if memSizeMib != 0 {
		vm.MemSizeMib = memSizeMib
	}
	if vCPUCount != 0 {
		vm.VCPUCount = vCPUCount
	}

	defer func() {
		// Free the VM from the pool if function returns error
		if retErr != nil {
			if err := o.vmPool.Free(vmID); err != nil {
				logger.WithError(err).Errorf("failed to free VM from pool after failure")
			}
		}
	}()

	ctx = namespaces.WithNamespace(ctx, NamespaceName)

	// 2. Fetch VM image
	tStart = time.Now()
	if vm.Image, err = o.GetImage(ctx, imageName); err != nil {
		return nil, nil, errors.Wrapf(err, "Failed to get/pull image")
	}
	startVMMetric.MetricMap[metrics.GetImage] = metrics.ToUS(time.Since(tStart))

	//===========================same as loadvm
	// 3. Create VM
	tStart = time.Now()
	conf := o.getVMConfig(vm, trackDirtyPages, o.isFullLocal)
	resp, err := o.fcClient.CreateVM(ctx, conf)
	startVMMetric.MetricMap[metrics.FcCreateVM] = metrics.ToUS(time.Since(tStart))
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create the microVM in firecracker-containerd")
	}

	defer func() {
		if retErr != nil {
			if _, err := o.fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID}); err != nil {
				logger.WithError(err).Errorf("failed to stop firecracker-containerd VM after failure")
			}
		}
	}()

	log.Debug(vm.GetNetworkNamespace())

	// 4. Create container
	logger.Debug("StartVM: Creating a new container")
	tStart = time.Now()

	containerId := vmID
	if o.isFullLocal {
		containerId = vm.ContainerSnapKey
	}

	container, err := o.client.NewContainer(
		ctx,
		containerId,
		containerd.WithSnapshotter(o.snapshotter),
		containerd.WithNewSnapshot(containerId, *vm.Image),
		containerd.WithNewSpec(
			oci.WithImageConfig(*vm.Image),
			firecrackeroci.WithVMID(vmID),
			firecrackeroci.WithVMNetwork,
		),
		containerd.WithRuntime("aws.firecracker", nil),
	)
	startVMMetric.MetricMap[metrics.NewContainer] = metrics.ToUS(time.Since(tStart))
	vm.Container = &container
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create a container")
	}

	defer func() {
		if retErr != nil {
			if err := container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
				logger.WithError(err).Errorf("failed to delete container after failure")
			}
		}
	}()

	// 5. Turn container into runnable process
	iologger := NewWorkloadIoWriter(vmID)
	o.workloadIo.Store(vmID, &iologger)
	logger.Debug("StartVM: Creating a new task")
	tStart = time.Now()
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStreams(os.Stdin, iologger, iologger)))
	startVMMetric.MetricMap[metrics.NewTask] = metrics.ToUS(time.Since(tStart))
	vm.Task = &task
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to create a task")
	}

	defer func() {
		if retErr != nil {
			if _, err := task.Delete(ctx); err != nil {
				logger.WithError(err).Errorf("failed to delete task after failure")
			}
		}
	}()

	// 6. Wait for task to get ready
	logger.Debug("StartVM: Waiting for the task to get ready")
	tStart = time.Now()
	ch, err := task.Wait(ctx)
	startVMMetric.MetricMap[metrics.TaskWait] = metrics.ToUS(time.Since(tStart))
	vm.TaskCh = ch
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to wait for a task")
	}

	defer func() {
		if retErr != nil {
			if err := task.Kill(ctx, syscall.SIGKILL); err != nil {
				logger.WithError(err).Errorf("failed to kill task after failure")
			}
		}
	}()

	// 7. Start process inside container
	logger.Debug("StartVM: Starting the task")
	tStart = time.Now()
	if err := task.Start(ctx); err != nil {
		return nil, nil, errors.Wrap(err, "failed to start a task")
	}
	startVMMetric.MetricMap[metrics.TaskStart] = metrics.ToUS(time.Since(tStart))

	defer func() {
		if retErr != nil {
			if err := task.Kill(ctx, syscall.SIGKILL); err != nil {
				logger.WithError(err).Errorf("failed to kill task after failure")
			}
		}
	}()

	if !o.isFullLocal {
		if err := os.MkdirAll(o.getVMBaseDir(vmID), 0777); err != nil {
			logger.Error("Failed to create VM base dir")
			return nil, nil, err
		}
		if o.GetUPFEnabled() {
			logger.Debug("Registering VM with the memory manager")

			stateCfg := manager.SnapshotStateCfg{
				VMID:             vmID,
				GuestMemPath:     o.getMemoryFile(vmID),
				BaseDir:          o.getVMBaseDir(vmID),
				GuestMemSize:     int(conf.MachineCfg.MemSizeMib) * 1024 * 1024,
				IsLazyMode:       o.isLazyMode,
				VMMStatePath:     o.getSnapshotFile(vmID),
				WorkingSetPath:   o.getWorkingSetFile(vmID),
				InstanceSockAddr: resp.UPFSockPath,
			}
			if err := o.memoryManager.RegisterVM(stateCfg); err != nil {
				return nil, nil, errors.Wrap(err, "failed to register VM with memory manager")
				// NOTE (Plamen): Potentially need a defer(DeregisteVM) here if RegisterVM is not last to execute
			}
		}
	}

	logger.Debug("Successfully started a VM")

	return &StartVMResponse{GuestIP: vm.GetIP()}, startVMMetric, nil
}

// StopSingleVM Shuts down a VM
// Note: VMs are not quisced before being stopped
func (o *Orchestrator) StopSingleVM(ctx context.Context, vmID string) error {

	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Orchestrator received StopVM")

	ctx = namespaces.WithNamespace(ctx, NamespaceName)
	vm, err := o.vmPool.GetVM(vmID)
	if err != nil {
		if _, ok := err.(*misc.NonExistErr); ok {
			logger.Panic("StopVM: VM does not exist")
		}
		logger.Panic("StopVM: GetVM() failed for an unknown reason")

	}

	logger = log.WithFields(log.Fields{"vmID": vmID})

	// Cleanup and remove container if VM not booted from snapshot
	if !o.isFullLocal || !vm.SnapBooted {
		task := *vm.Task
		if err := task.Kill(ctx, syscall.SIGKILL); err != nil {
			logger.WithError(err).Error("Failed to kill the task")
			return err
		}

		<-vm.TaskCh
		//FIXME: Seems like some tasks need some extra time to die Issue#15, lr_training
		time.Sleep(500 * time.Millisecond)

		if _, err := task.Delete(ctx); err != nil {
			logger.WithError(err).Error("failed to delete task")
			return err
		}

		container := *vm.Container
		if err := container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
			logger.WithError(err).Error("failed to delete container")
			return err
		}
	}

	// this doesnt break but leaves containers loaded from remote in unkown state for k8/knative
	//if !o.isFullLocal || vm.RemoteSnapBooted {
	//	task := *vm.Task
	//	if err := task.Kill(ctx, syscall.SIGKILL); err != nil {
	//		logger.WithError(err).Error("Failed to kill the task")
	//		return err
	//	}
	//
	//	<-vm.TaskCh
	//	//FIXME: Seems like some tasks need some extra time to die Issue#15, lr_training
	//	time.Sleep(500 * time.Millisecond)
	//
	//	if _, err := task.Delete(ctx); err != nil {
	//		logger.WithError(err).Error("failed to delete task")
	//		return err
	//	}
	//
	//	container := *vm.Container
	//	if err := container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
	//		logger.WithError(err).Error("failed to delete container")
	//		return err
	//	}
	//}

	// Stop VM
	if _, err := o.fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID}); err != nil {
		logger.WithError(err).Error("failed to stop firecracker-containerd VM")
		return err
	}

	// Free VM metadata and clean up network
	if err := o.vmPool.Free(vmID); err != nil {
		logger.Error("failed to free VM from VM pool")
		return err
	}

	o.workloadIo.Delete(vmID)

	// Cleanup VM devmapper container snapshot if booted from snapshot
	if o.isFullLocal && vm.SnapBooted {
		if err := o.devMapper.RemoveDeviceSnapshot(ctx, vm.ContainerSnapKey); err != nil {
			logger.Error("failed to deactivate container snapshot")
			return err
		}
	}

	//if o.isFullLocal && vm.RemoteSnapBooted {
	//	if err := o.devMapper.RemoveDeviceSnapshot(ctx, vm.ContainerSnapKey); err != nil {
	//		logger.Error("failed to deactivate container snapshot")
	//		return err
	//	}
	//}

	logger.Debug("Stopped VM successfully")

	return nil
}

func (o *Orchestrator) GetImage(ctx context.Context, imageName string) (*containerd.Image, error) {
	return o.imageManager.GetImage(ctx, imageName)
}

func getK8sDNS() []string {
	//using googleDNS as a backup
	dnsIPs := []string{"8.8.8.8"}
	//get k8s DNS clusterIP
	cmd := exec.Command(
		"kubectl", "get", "service", "-n", "kube-system", "kube-dns", "-o=custom-columns=:.spec.clusterIP", "--no-headers",
	)
	stdoutStderr, err := cmd.CombinedOutput()
	if err != nil {
		log.Warnf("Failed to Fetch k8s dns clusterIP %v\n%s\n", err, stdoutStderr)
		log.Warnf("Using google dns %s\n", dnsIPs[0])
	} else {
		//adding k8s DNS clusterIP to the list
		dnsIPs = []string{strings.TrimSpace(string(stdoutStderr)), dnsIPs[0]}
	}
	return dnsIPs
}

func (o *Orchestrator) getVMConfig(vm *misc.VM, trackDirtyPages, isFullLocal bool) *proto.CreateVMRequest {
	kernelArgs := "ro noapic reboot=k panic=1 pci=off nomodules systemd.log_color=false systemd.unit=firecracker.target init=/sbin/overlay-init tsc=reliable quiet 8250.nr_uarts=0 ipv6.disable=1"

	return &proto.CreateVMRequest{
		VMID:           vm.ID,
		TimeoutSeconds: 100,
		KernelArgs:     kernelArgs,
		MachineCfg: &proto.FirecrackerMachineConfiguration{
			VcpuCount:       vm.VCPUCount,
			MemSizeMib:      vm.MemSizeMib,
			TrackDirtyPages: trackDirtyPages,
		},
		NetworkInterfaces: []*proto.FirecrackerNetworkInterface{{
			StaticConfig: &proto.StaticNetworkConfiguration{
				MacAddress:  vm.GetMacAddress(),
				HostDevName: vm.GetHostDevName(),
				IPConfig: &proto.IPConfiguration{
					PrimaryAddr: vm.GetPrimaryAddr(),
					GatewayAddr: vm.GetGatewayAddr(),
					Nameservers: getK8sDNS(),
				},
			},
		}},
		NetworkNamespace: vm.GetNetworkNamespace(),
		OffloadEnabled:   !isFullLocal,
	}
}

// StopActiveVMs Shuts down all active VMs
func (o *Orchestrator) StopActiveVMs() error {
	var vmGroup sync.WaitGroup
	for vmID, vm := range o.vmPool.GetVMMap() {
		vmGroup.Add(1)
		logger := log.WithFields(log.Fields{"vmID": vmID})
		go func(vmID string, vm *misc.VM) {
			defer vmGroup.Done()
			err := o.StopSingleVM(context.Background(), vmID)
			if err != nil {
				logger.Warn(err)
			}
		}(vmID, vm)
	}

	log.Info("waiting for goroutines")
	vmGroup.Wait()
	log.Info("waiting done")

	log.Info("Closing fcClient")
	defer func() { _ = o.fcClient.Close() }()
	log.Info("Closing containerd client")
	defer func() { _ = o.client.Close() }()

	return nil
}

// PauseVM Pauses a VM
func (o *Orchestrator) PauseVM(ctx context.Context, vmID string) error {
	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Orchestrator received PauseVM")

	ctx = namespaces.WithNamespace(ctx, NamespaceName)

	if _, err := o.fcClient.PauseVM(ctx, &proto.PauseVMRequest{VMID: vmID}); err != nil {
		logger.WithError(err).Error("failed to pause the VM")
		return err
	}

	return nil
}

// ResumeVM Resumes a VM
func (o *Orchestrator) ResumeVM(ctx context.Context, vmID string) (*metrics.Metric, error) {
	var (
		resumeVMMetric *metrics.Metric = metrics.NewMetric()
		tStart         time.Time
	)

	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Orchestrator received ResumeVM")

	ctx = namespaces.WithNamespace(ctx, NamespaceName)

	tStart = time.Now()
	if _, err := o.fcClient.ResumeVM(ctx, &proto.ResumeVMRequest{VMID: vmID}); err != nil {
		logger.WithError(err).Error("failed to resume the VM")
		return nil, err
	}
	resumeVMMetric.MetricMap[metrics.FcResume] = metrics.ToUS(time.Since(tStart))

	return resumeVMMetric, nil
}

// CreateSnapshot Creates a snapshot of a VM
func (o *Orchestrator) CreateSnapshot(ctx context.Context, vmID string, snap *snapshotting.Snapshot) error {
	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Orchestrator received CreateSnapshot")

	ctx = namespaces.WithNamespace(ctx, NamespaceName)

	// 1. Create VM & VM memory state snapshot
	snapFilePath := o.getSnapshotFile(vmID)
	memFilePath := o.getMemoryFile(vmID)

	if o.isFullLocal {
		snapFilePath = snap.GetSnapFilePath()
		memFilePath = snap.GetMemFilePath()
	}

	req := &proto.CreateSnapshotRequest{
		VMID:             vmID,
		SnapshotFilePath: snapFilePath,
		MemFilePath:      memFilePath,
		SnapshotType:     snap.GetSnapType(),
	}

	if _, err := o.fcClient.CreateSnapshot(ctx, req); err != nil {
		logger.WithError(err).Error("failed to create snapshot of the VM")
		return err
	}

	// For the non full-local snapshots, no additional steps are necessary
	if !o.isFullLocal {
		return nil
	}

	// 2. Get VM metadata
	vm, err := o.vmPool.GetVM(vmID)
	if err != nil {
		return err
	}

	// 3. Backup disk state difference.
	// 3.B Alternatively could also do ForkContainerSnap(ctx, vm.ContainerSnapKey, snap.GetContainerSnapName(), *vm.Image, forkMetric)
	patchFilePath := snap.GetPatchFilePath()
	logger = log.WithFields(log.Fields{"vmID": vmID, "patchFilePath": patchFilePath})
	logger.Debug("Creating patch file with disk state difference")

	if err := o.devMapper.CreatePatch(ctx, patchFilePath, vm.ContainerSnapKey, *vm.Image); err != nil {
		logger.WithError(err).Error("failed to create container patch file")
		return err
	}

	// 4. Serialize snapshot info
	logger = log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Serializing snapshot info")
	if err := snap.SerializeSnapInfo(); err != nil {
		logger.WithError(err).Error("failed to serialize snapshot info")
		return err
	}

	//// 5. Save patch file, uVM memory and state to minio storage
	//log.Debug("Save patch file to minio storage")
	//ctx = context.Background()
	//minioEndpoint := "10.96.0.46:9000"
	//minioAccessKey := "minio"
	//minioSceretKey := "minio123"
	//minioBucket := "mybucket"
	//
	//log.Debug("Creating minio client")
	//// Initialize minio client object.
	//minioClient, err := minio.New(minioEndpoint, &minio.Options{
	//	Creds:  credentials.NewStaticV4(minioAccessKey, minioSceretKey, ""),
	//	Secure: false,
	//})
	//if err != nil {
	//	log.Debug(err)
	//}
	//
	//log.Debug("Checking if bucket exists \n")
	//err = minioClient.MakeBucket(ctx, minioBucket, minio.MakeBucketOptions{})
	//if err != nil {
	//	// Check to see if bucket exists	<-- we assume we create the bucket
	//	// in the script, so we don't  need to take care of this in the code but just
	//	// in case
	//	exists, errBucketExists := minioClient.BucketExists(ctx, minioBucket)
	//	if errBucketExists == nil && exists {
	//		log.Debug("We already own %s\n", minioBucket)
	//	} else {
	//		log.Debug(err)
	//	}
	//} else {
	//	log.Debug("Successfully created %s\n", minioBucket)
	//}
	//
	//ctx = namespaces.WithNamespace(ctx, NamespaceName)
	//fctImage, _ := (*vm.Container).Image(ctx)
	//objectPatch := fctImage.Name() + "PatchFile"
	//objectSnap := fctImage.Name() + "SnapFile"
	//objectMemory := fctImage.Name() + "MemFile"
	//filePathPatch := snap.GetPatchFilePath()
	//contentType := "application/octet-stream"
	//
	//// Change the rights of mem and snapshot files to enable upload to local storage
	//_, errSnap := os.Stat(snapFilePath)
	//_, errMem := os.Stat(memFilePath)
	//
	//if errSnap == nil && errMem == nil {
	//
	//	_ = exec.Command("sudo", "chmod", "777", snapFilePath)
	//	_ = exec.Command("sudo", "chmod", "777", memFilePath)
	//
	//	// Upload patch file
	//	infoPut, errPut := minioClient.FPutObject(ctx, minioBucket, objectPatch, filePathPatch, minio.PutObjectOptions{ContentType: contentType})
	//	if errPut != nil {
	//		log.WithError(errPut).Error("failed to upload patchfile to minio")
	//	}
	//
	//	log.Debug("Successfully uploaded %s of size %d\n", objectPatch, infoPut.Size)
	//
	//	// upload snapshot file
	//	infoPut, errPut = minioClient.FPutObject(ctx, minioBucket, objectSnap, snapFilePath, minio.PutObjectOptions{ContentType: contentType})
	//	if errPut != nil {
	//		log.WithError(errPut).Error("failed to upload snapfile to minio")
	//	}
	//
	//	log.Debug("Successfully uploaded %s of size %d\n", objectSnap, infoPut.Size)
	//
	//	// Upload mem file
	//	infoPut, errPut = minioClient.FPutObject(ctx, minioBucket, objectMemory, memFilePath, minio.PutObjectOptions{ContentType: contentType})
	//	if errPut != nil {
	//		log.WithError(errPut).Error("failed to upload memfile to minio")
	//	}
	//
	//	log.Debug("Successfully uploaded %s of size %d\n", objectMemory, infoPut.Size)
	//
	//}
	return nil
}

// LoadSnapshot Loads a snapshot of a VM
func (o *Orchestrator) LoadSnapshot(
	ctx context.Context,
	vmID string,
	snap *snapshotting.Snapshot) (_ *StartVMResponse, _ *metrics.Metric, retErr error) {

	var (
		loadSnapshotMetric   *metrics.Metric = metrics.NewMetric()
		tStart               time.Time
		loadErr, activateErr error
		loadDone             = make(chan int)
	)

	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Orchestrator received LoadSnapshot")

	ctx = namespaces.WithNamespace(ctx, NamespaceName)

	var containerSnap *devmapper.DeviceSnapshot
	var vm *misc.VM
	if o.isFullLocal {
		var err error

		// 1. Allocate VM metadata & create vm network
		vm, err = o.vmPool.Allocate(vmID)
		if err != nil {
			logger.Error("failed to allocate VM in VM pool")
			return nil, nil, err
		}

		defer func() {
			// Free the VM from the pool if function returns error
			if retErr != nil {
				if err := o.vmPool.Free(vmID); err != nil {
					logger.WithError(err).Errorf("failed to free VM from pool after failure")
				}
			}
		}()

		// 2. Fetch image for VM
		if vm.Image, err = o.GetImage(ctx, snap.GetImage()); err != nil {
			return nil, nil, errors.Wrapf(err, "Failed to get/pull image")
		}

		//////////////////=================same as start vm
		// 3. Create snapshot for container to run
		// 3.B Alternatively could also do CreateDeviceSnapshot(ctx, vm.ContainerSnapKey, snap.GetContainerSnapName())
		if err := o.devMapper.CreateDeviceSnapshotFromImage(ctx, vm.ContainerSnapKey, *vm.Image); err != nil {
			return nil, nil, errors.Wrapf(err, "creating container snapshot")
		}

		containerSnap, err = o.devMapper.GetDeviceSnapshot(ctx, vm.ContainerSnapKey)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "previously created container device does not exist")
		}

		// 4. Unpack patch into container snapshot
		if err := o.devMapper.RestorePatch(ctx, vm.ContainerSnapKey, snap.GetPatchFilePath()); err != nil {
			return nil, nil, errors.Wrapf(err, "unpacking patch into container snapshot")
		}
	} else {
		var err error
		vm, err = o.vmPool.GetVM(vmID)
		if err != nil {
			return nil, nil, err
		}
	}

	// 5. Load VM from snapshot
	snapFilePath := o.getSnapshotFile(vmID)
	memFilePath := o.getMemoryFile(vmID)

	if o.isFullLocal {
		snapFilePath = snap.GetSnapFilePath()
		memFilePath = snap.GetMemFilePath()
	}

	req := &proto.LoadSnapshotRequest{
		VMID:             vmID,
		SnapshotFilePath: snapFilePath,
		MemFilePath:      memFilePath,
		EnableUserPF:     o.GetUPFEnabled(),
		NetworkNamespace: "",
		Offloaded:        !o.isFullLocal,
	}

	if o.isFullLocal {
		req.NewSnapshotPath = containerSnap.GetDevicePath()
		req.NetworkNamespace = vm.GetNetworkNamespace()
	}

	if o.GetUPFEnabled() {
		if err := o.memoryManager.FetchState(vmID); err != nil {
			return nil, nil, err
		}
	}

	tStart = time.Now()

	go func() {
		defer close(loadDone)

		if _, loadErr = o.fcClient.LoadSnapshot(ctx, req); loadErr != nil {
			logger.Error("Failed to load snapshot of the VM: ", loadErr)
			logger.Errorf("snapFilePath: %s, memFilePath: %s, newSnapshotPath: %s", snapFilePath, memFilePath, containerSnap.GetDevicePath())
			files, err := ioutil.ReadDir(filepath.Dir(snapFilePath))
			if err != nil {
				logger.Error(err)
			}

			snapFiles := ""
			for _, f := range files {
				snapFiles += f.Name() + ", "
			}

			logger.Error(snapFiles)

			files, _ = ioutil.ReadDir(filepath.Dir(containerSnap.GetDevicePath()))
			if err != nil {
				logger.Error(err)
			}

			snapFiles = ""
			for _, f := range files {
				snapFiles += f.Name() + ", "
			}
			logger.Error(snapFiles)
		}
	}()

	if o.GetUPFEnabled() {
		if activateErr = o.memoryManager.Activate(vmID); activateErr != nil {
			logger.Warn("Failed to activate VM in the memory manager", activateErr)
		}
	}

	<-loadDone

	loadSnapshotMetric.MetricMap[metrics.LoadVMM] = metrics.ToUS(time.Since(tStart))

	if loadErr != nil || activateErr != nil {
		multierr := multierror.Of(loadErr, activateErr)
		return nil, nil, multierr
	}

	vm.SnapBooted = true

	return &StartVMResponse{GuestIP: vm.GetIP()}, loadSnapshotMetric, nil
}

// RemoteLoadSnapshot Loads a snapshot of a VM from remote storage
func (o *Orchestrator) RemoteLoadSnapshot(
	ctx context.Context,
	vmID string,
	image string,
	filePathPatch string,
	filePathSnap string,
	filePathMem string) (_ *StartVMResponse, _ *metrics.Metric, retErr error) {

	var (
		loadSnapshotMetric   *metrics.Metric = metrics.NewMetric()
		tStart               time.Time
		loadErr, activateErr error
		loadDone             = make(chan int)
	)

	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Orchestrator received RemoteLoadSnapshot")

	ctx = namespaces.WithNamespace(ctx, NamespaceName)

	var containerSnap *devmapper.DeviceSnapshot
	var vm *misc.VM
	if o.isFullLocal {
		var err error

		// 1. Allocate VM metadata & create vm network
		vm, err = o.vmPool.Allocate(vmID)
		if err != nil {
			logger.Error("failed to allocate VM in VM pool")
			return nil, nil, err
		}

		defer func() {
			// Free the VM from the pool if function returns error
			if retErr != nil {
				if err := o.vmPool.Free(vmID); err != nil {
					logger.WithError(err).Errorf("failed to free VM from pool after failure")
				}
			}
		}()

		// 2. Fetch image for VM
		vm.Image, err = o.GetImage(ctx, image)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "Failed to get/pull image")
		}

		//////////////////=================same as start vm
		// 3. Create snapshot for container to run
		// 3.B Alternatively could also do CreateDeviceSnapshot(ctx, vm.ContainerSnapKey, snap.GetContainerSnapName())
		if err := o.devMapper.CreateDeviceSnapshotFromImage(ctx, vm.ContainerSnapKey, *vm.Image); err != nil {
			return nil, nil, errors.Wrapf(err, "creating container snapshot")
		}

		containerSnap, err = o.devMapper.GetDeviceSnapshot(ctx, vm.ContainerSnapKey)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "previously created container device does not exist")
		}

		// 4. Unpack patch into container snapshot
		if err := o.devMapper.RestorePatch(ctx, vm.ContainerSnapKey, filePathPatch); err != nil {
			return nil, nil, errors.Wrapf(err, "unpacking patch into container snapshot")
		}
	} else {
		var err error
		vm, err = o.vmPool.GetVM(vmID)
		if err != nil {
			return nil, nil, err
		}
	}

	// 5. Load VM from snapshot
	req := &proto.LoadSnapshotRequest{
		VMID:             vmID,
		SnapshotFilePath: filePathSnap,
		MemFilePath:      filePathMem,
		EnableUserPF:     o.GetUPFEnabled(),
		NetworkNamespace: "",
		Offloaded:        !o.isFullLocal,
	}

	if o.isFullLocal {
		req.NewSnapshotPath = containerSnap.GetDevicePath()
		req.NetworkNamespace = vm.GetNetworkNamespace()
		log.Debug(vm.GetNetworkNamespace())

	}

	if o.GetUPFEnabled() {
		if err := o.memoryManager.FetchState(vmID); err != nil {
			return nil, nil, err
		}
	}

	tStart = time.Now()

	go func() {
		defer close(loadDone)

		if _, loadErr = o.fcClient.LoadSnapshot(ctx, req); loadErr != nil {
			logger.Error("Failed to load snapshot of the VM: ", loadErr)
			logger.Errorf("snapFilePath: %s, memFilePath: %s, newSnapshotPath: %s", filePathSnap, filePathMem, containerSnap.GetDevicePath())
			files, err := ioutil.ReadDir(filepath.Dir(filePathSnap))
			if err != nil {
				logger.Error(err)
			}

			snapFiles := ""
			for _, f := range files {
				snapFiles += f.Name() + ", "
			}

			logger.Error(snapFiles)

			files, _ = ioutil.ReadDir(filepath.Dir(containerSnap.GetDevicePath()))
			if err != nil {
				logger.Error(err)
			}

			snapFiles = ""
			for _, f := range files {
				snapFiles += f.Name() + ", "
			}
			logger.Error(snapFiles)
		}
	}()

	if o.GetUPFEnabled() {
		if activateErr = o.memoryManager.Activate(vmID); activateErr != nil {
			logger.Warn("Failed to activate VM in the memory manager", activateErr)
		}
	}

	<-loadDone

	loadSnapshotMetric.MetricMap[metrics.LoadVMM] = metrics.ToUS(time.Since(tStart))

	if loadErr != nil || activateErr != nil {
		multierr := multierror.Of(loadErr, activateErr)
		return nil, nil, multierr
	}

	vm.SnapBooted = true
	vm.RemoteSnapBooted = true

	return &StartVMResponse{GuestIP: vm.GetIP()}, loadSnapshotMetric, nil
}

// RemoteLoadSnapshot Loads a snapshot of a VM from remote storage
//func (o *Orchestrator) RemoteCreateContainerSnapshot(
//	ctx context.Context,
//	vmID string,
//	image string,
//	filePathPatch string,
//	filePathSnap string,
//	filePathMem string) (_ *StartVMResponse, _ *metrics.Metric, retErr error) {
//
//	// 4. Create container
//
//	containerId := vmID
//	if o.isFullLocal {
//		containerId = vm.ContainerSnapKey
//	}
//
//	container, err := o.client.NewContainer(
//		ctx,
//		containerId,
//		containerd.WithSnapshotter(o.snapshotter),
//		containerd.WithNewSnapshot(containerId, *vm.Image),
//		containerd.WithNewSpec(
//			oci.WithImageConfig(*vm.Image),
//			firecrackeroci.WithVMID(vmID),
//			firecrackeroci.WithVMNetwork,
//		),
//		containerd.WithRuntime("aws.firecracker", nil),
//	)
//
//	vm.Container = &container
//	if err != nil {
//		return nil, nil, errors.Wrap(err, "failed to create a container")
//	}
//
//	defer func() {
//		if retErr != nil {
//			if err := container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
//				logger.WithError(err).Errorf("failed to delete container after failure")
//			}
//		}
//	}()
//
//	// 5. Turn container into runnable process
//	iologger := NewWorkloadIoWriter(vmID)
//	o.workloadIo.Store(vmID, &iologger)
//	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStreams(os.Stdin, iologger, iologger)))
//	vm.Task = &task
//	if err != nil {
//		return nil, nil, errors.Wrapf(err, "failed to create a task")
//	}
//
//	defer func() {
//		if retErr != nil {
//			if _, err := task.Delete(ctx); err != nil {
//				logger.WithError(err).Errorf("failed to delete task after failure")
//			}
//		}
//	}()
//
//	// 6. Wait for task to get ready
//	logger.Debug("StartVM: Waiting for the task to get ready")
//	tStart = time.Now()
//	ch, err := task.Wait(ctx)
//	startVMMetric.MetricMap[metrics.TaskWait] = metrics.ToUS(time.Since(tStart))
//	vm.TaskCh = ch
//	if err != nil {
//		return nil, nil, errors.Wrap(err, "failed to wait for a task")
//	}
//
//	defer func() {
//		if retErr != nil {
//			if err := task.Kill(ctx, syscall.SIGKILL); err != nil {
//				logger.WithError(err).Errorf("failed to kill task after failure")
//			}
//		}
//	}()
//
//	// 7. Start process inside container
//	logger.Debug("StartVM: Starting the task")
//	tStart = time.Now()
//	if err := task.Start(ctx); err != nil {
//		return nil, nil, errors.Wrap(err, "failed to start a task")
//	}
//	startVMMetric.MetricMap[metrics.TaskStart] = metrics.ToUS(time.Since(tStart))
//
//	defer func() {
//		if retErr != nil {
//			if err := task.Kill(ctx, syscall.SIGKILL); err != nil {
//				logger.WithError(err).Errorf("failed to kill task after failure")
//			}
//		}
//	}()
//
//	if !o.isFullLocal {
//		if err := os.MkdirAll(o.getVMBaseDir(vmID), 0777); err != nil {
//			logger.Error("Failed to create VM base dir")
//			return nil, nil, err
//		}
//		if o.GetUPFEnabled() {
//			logger.Debug("Registering VM with the memory manager")
//
//			stateCfg := manager.SnapshotStateCfg{
//				VMID:             vmID,
//				GuestMemPath:     o.getMemoryFile(vmID),
//				BaseDir:          o.getVMBaseDir(vmID),
//				GuestMemSize:     int(conf.MachineCfg.MemSizeMib) * 1024 * 1024,
//				IsLazyMode:       o.isLazyMode,
//				VMMStatePath:     o.getSnapshotFile(vmID),
//				WorkingSetPath:   o.getWorkingSetFile(vmID),
//				InstanceSockAddr: resp.UPFSockPath,
//			}
//			if err := o.memoryManager.RegisterVM(stateCfg); err != nil {
//				return nil, nil, errors.Wrap(err, "failed to register VM with memory manager")
//				// NOTE (Plamen): Potentially need a defer(DeregisteVM) here if RegisterVM is not last to execute
//			}
//		}
//	}
//
//	logger.Debug("Successfully started a VM")
//
//	return
//}

// Offload Shuts down the VM but leaves shim and other resources running.
func (o *Orchestrator) OffloadVM(ctx context.Context, vmID string) error {
	if o.isFullLocal {
		return errors.New("Fully local snapshots do not support offloading")
	}

	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Orchestrator received Offload")

	ctx = namespaces.WithNamespace(ctx, NamespaceName)

	_, err := o.vmPool.GetVM(vmID)
	if err != nil {
		if _, ok := err.(*misc.NonExistErr); ok {
			logger.Panic("Offload: VM does not exist")
		}
		logger.Panic("Offload: GetVM() failed for an unknown reason")

	}

	if o.GetUPFEnabled() {
		if err := o.memoryManager.Deactivate(vmID); err != nil {
			logger.Error("Failed to deactivate VM in the memory manager")
			return err
		}
	}

	if _, err := o.fcClient.Offload(ctx, &proto.OffloadRequest{VMID: vmID}); err != nil {
		logger.WithError(err).Error("failed to offload the VM")
		return err
	}

	if err := o.vmPool.RecreateTap(vmID); err != nil {
		logger.Error("Failed to recreate tap upon offloading")
		return err
	}

	return nil
}

// CleanupSnapshot removes a devicemapper snapshot. This function is only necessary if the alternative approach with
// thin-delta is used. Otherwise, snapshots created from within vHive get already cleaned up during stopVM.
func (o *Orchestrator) CleanupSnapshot(ctx context.Context, revisionID string) error {
	if err := o.devMapper.RemoveDeviceSnapshot(ctx, revisionID); err != nil {
		return errors.Wrapf(err, "removing revision snapshot")
	}
	return nil
}
