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

package deduplicated

import (
	"context"
	"github.com/ease-lab/vhive/ctriface"
	"github.com/ease-lab/vhive/snapshotting"
	"os"
	"os/exec"
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

	"github.com/ease-lab/vhive/metrics"
	"github.com/ease-lab/vhive/misc"
	"github.com/go-multierror/multierror"

	_ "github.com/davecgh/go-spew/spew" //tmp
)

// StartVM Boots a VM if it does not exist
func (o *DedupOrchestrator) StartVM(ctx context.Context, vmID, imageName string, memSizeMib ,vCPUCount uint32, trackDirtyPages bool) (_ *ctriface.StartVMResponse, _ *metrics.Metric, retErr error) {
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

	ctx = namespaces.WithNamespace(ctx, namespaceName)

	// 2. Fetch VM image
	tStart = time.Now()
	if vm.Image, err = o.GetImage(ctx, imageName); err != nil {
		return nil, nil, errors.Wrapf(err, "Failed to get/pull image")
	}
	startVMMetric.MetricMap[metrics.GetImage] = metrics.ToUS(time.Since(tStart))

	// 3. Create VM
	tStart = time.Now()
	conf := o.getVMConfig(vm, trackDirtyPages)
	_, err = o.fcClient.CreateVM(ctx, conf)
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

	// 4. Create container
	logger.Debug("StartVM: Creating a new container")
	tStart = time.Now()
	container, err := o.client.NewContainer(
		ctx,
		vm.ContainerSnapKey,
		containerd.WithSnapshotter(o.snapshotter),
		containerd.WithNewSnapshot(vm.ContainerSnapKey, *vm.Image),
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

	logger.Debug("Successfully started a VM")

	return &ctriface.StartVMResponse{GuestIP: vm.NetConfig.GetCloneIP()}, startVMMetric, nil
}

// StopSingleVM Shuts down a VM
// Note: VMs are not quisced before being stopped
func (o *DedupOrchestrator) StopSingleVM(ctx context.Context, vmID string) error {
	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("DedupOrchestrator received StopVM")

	ctx = namespaces.WithNamespace(ctx, namespaceName)
	vm, err := o.vmPool.GetVM(vmID)
	if err != nil {
		if _, ok := err.(*misc.NonExistErr); ok {
			logger.Panic("StopVM: VM does not exist")
		}
		logger.Panic("StopVM: GetVM() failed for an unknown reason")

	}

	logger = log.WithFields(log.Fields{"vmID": vmID})

	// Cleanup and remove container if VM not booted from snapshot
	if ! vm.SnapBooted {
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
	if vm.SnapBooted {
		if err := o.devMapper.RemoveDeviceSnapshot(ctx, vm.ContainerSnapKey); err != nil {
			logger.Error("failed to deactivate container snapshot")
			return err
		}
	}

	logger.Debug("Stopped VM successfully")

	return nil
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

func (o *DedupOrchestrator) getVMConfig(vm *misc.VM, trackDirtyPages bool) *proto.CreateVMRequest {
	kernelArgs := "ro noapic reboot=k panic=1 pci=off nomodules systemd.log_color=false systemd.unit=firecracker.target init=/sbin/overlay-init tsc=reliable quiet 8250.nr_uarts=0 ipv6.disable=1"

	return &proto.CreateVMRequest{
		VMID:           vm.ID,
		TimeoutSeconds: 100,
		KernelArgs:     kernelArgs,
		MachineCfg: &proto.FirecrackerMachineConfiguration{
			VcpuCount:  vm.VCPUCount,
			MemSizeMib: vm.MemSizeMib,
			TrackDirtyPages: trackDirtyPages,
		},
		NetworkInterfaces: []*proto.FirecrackerNetworkInterface{{
			StaticConfig: &proto.StaticNetworkConfiguration{
				MacAddress:  vm.NetConfig.GetMacAddress(),
				HostDevName: vm.NetConfig.GetHostDevName(),
				IPConfig: &proto.IPConfiguration{
					PrimaryAddr: vm.NetConfig.GetContainerCIDR(),
					GatewayAddr: vm.NetConfig.GetGatewayIP(),
					Nameservers: getK8sDNS(),
				},
			},
		}},
		NetworkNamespace: vm.NetConfig.GetNamespacePath(),
		OffloadEnabled: false,
	}
}

// Offload Shuts down the VM but leaves shim and other resources running.
func (o *DedupOrchestrator) OffloadVM(ctx context.Context, vmID string) error {
	return errors.New("Deduplicated snapshots do not support offloading")
}

// StopActiveVMs Shuts down all active VMs
func (o *DedupOrchestrator) StopActiveVMs() error {
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
	o.fcClient.Close()
	log.Info("Closing containerd client")
	o.client.Close()

	return nil
}

// PauseVM Pauses a VM
func (o *DedupOrchestrator) PauseVM(ctx context.Context, vmID string) error {
	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("DedupOrchestrator received PauseVM")

	ctx = namespaces.WithNamespace(ctx, namespaceName)

	if _, err := o.fcClient.PauseVM(ctx, &proto.PauseVMRequest{VMID: vmID}); err != nil {
		logger.WithError(err).Error("failed to pause the VM")
		return err
	}

	return nil
}

// ResumeVM Resumes a VM
func (o *DedupOrchestrator) ResumeVM(ctx context.Context, vmID string) (*metrics.Metric, error) {
	var (
		resumeVMMetric *metrics.Metric = metrics.NewMetric()
		tStart         time.Time
	)

	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("DedupOrchestrator received ResumeVM")

	ctx = namespaces.WithNamespace(ctx, namespaceName)

	tStart = time.Now()
	if _, err := o.fcClient.ResumeVM(ctx, &proto.ResumeVMRequest{VMID: vmID}); err != nil {
		logger.WithError(err).Error("failed to resume the VM")
		return nil, err
	}
	resumeVMMetric.MetricMap[metrics.FcResume] = metrics.ToUS(time.Since(tStart))

	return resumeVMMetric, nil
}

// CreateSnapshot Creates a snapshot of a VM
func (o *DedupOrchestrator) CreateSnapshot(ctx context.Context, vmID string, snap *snapshotting.Snapshot) error {
	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("DedupOrchestrator received CreateSnapshot")

	ctx = namespaces.WithNamespace(ctx, namespaceName)

	// 1. Get VM metadata
	vm, err := o.vmPool.GetVM(vmID)
	if err != nil {
		return err
	}

	// 2. Create VM & VM memory state snapshot
	req := &proto.CreateSnapshotRequest{
		VMID:             vmID,
		SnapshotFilePath: snap.GetSnapFilePath(),
		MemFilePath:      snap.GetMemFilePath(),
		SnapshotType:     snap.GetSnapType(),
	}

	if _, err := o.fcClient.CreateSnapshot(ctx, req); err != nil {
		logger.WithError(err).Error("failed to create snapshot of the VM")
		return err
	}

	// 3. Backup disk state difference.
	// 3.B Alternatively could also do ForkContainerSnap(ctx, vm.ContainerSnapKey, snap.GetContainerSnapName(), *vm.Image, forkMetric)
	if err := o.devMapper.CreatePatch(ctx, snap.GetPatchFilePath(), vm.ContainerSnapKey, *vm.Image); err != nil {
		logger.WithError(err).Error("failed to create container patch file")
		return err
	}

	// 4. Serialize snapshot info
	if err := snap.SerializeSnapInfo(); err != nil {
		logger.WithError(err).Error("failed to serialize snapshot info")
		return err
	}

	// 5. Resume
	if _, err := o.fcClient.ResumeVM(ctx, &proto.ResumeVMRequest{VMID: vmID}); err != nil {
		log.Printf("failed to resume the VM")
		return  err
	}

	return nil
}

// LoadSnapshot Loads a snapshot of a VM
func (o *DedupOrchestrator) LoadSnapshot(ctx context.Context, vmID string, snap *snapshotting.Snapshot) (_ *ctriface.StartVMResponse, _ *metrics.Metric, retErr error) {
	var (
		loadSnapshotMetric   *metrics.Metric = metrics.NewMetric()
		tStart               time.Time
		loadErr, activateErr error
	)

	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("DedupOrchestrator received LoadSnapshot")

	ctx = namespaces.WithNamespace(ctx, namespaceName)

	// 1. Allocate VM metadata & create vm network
	vm, err := o.vmPool.Allocate(vmID)
	if err != nil {
		logger.Error("failed to allocate VM in VM pool")
		return nil, nil,  err
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
		return nil, nil,  errors.Wrapf(err, "Failed to get/pull image")
	}

	// 3. Create snapshot for container to run
	// 3.B Alternatively could also do CreateDeviceSnapshot(ctx, vm.ContainerSnapKey, snap.GetContainerSnapName())
	if err := o.devMapper.CreateDeviceSnapshotFromImage(ctx, vm.ContainerSnapKey, *vm.Image); err != nil {
		return nil, nil, errors.Wrapf(err, "creating container snapshot")
	}

	containerSnap, err := o.devMapper.GetDeviceSnapshot(ctx, vm.ContainerSnapKey)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "previously created container device does not exist")
	}

	// 4. Unpack patch into container snapshot
	if err := o.devMapper.RestorePatch(ctx, vm.ContainerSnapKey, snap.GetPatchFilePath()); err != nil {
		return nil, nil, errors.Wrapf(err, "unpacking patch into container snapshot")
	}

	// 5. Load VM from snapshot
	req := &proto.LoadSnapshotRequest{
		VMID:             vmID,
		SnapshotFilePath: snap.GetSnapFilePath(),
		MemFilePath:      snap.GetMemFilePath(),
		EnableUserPF:     false,
		NetworkNamespace: vm.NetConfig.GetNamespacePath(),
		NewSnapshotPath:  containerSnap.GetDevicePath(),
		Offloaded: false,
	}

	tStart = time.Now()

	if _, loadErr = o.fcClient.LoadSnapshot(ctx, req); loadErr != nil {
		logger.Error("Failed to load snapshot of the VM: ", loadErr)
	}

	loadSnapshotMetric.MetricMap[metrics.LoadVMM] = metrics.ToUS(time.Since(tStart))

	if loadErr != nil || activateErr != nil {
		multierr := multierror.Of(loadErr, activateErr)
		return nil, nil, multierr
	}

	vm.SnapBooted = true

	return  &ctriface.StartVMResponse{GuestIP: vm.NetConfig.GetCloneIP()}, nil, nil
}

func (o *DedupOrchestrator) CleanupSnapshot(ctx context.Context, revisionID string) error {
	if err := o.devMapper.RemoveDeviceSnapshot(ctx, revisionID); err != nil {
		return errors.Wrapf(err, "removing revision snapshot")
	}
	return nil
}

func (o *DedupOrchestrator) GetImage(ctx context.Context, imageName string) (*containerd.Image, error) {
	return o.imageManager.GetImage(ctx, imageName)
}
