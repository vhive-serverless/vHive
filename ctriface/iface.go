// MIT License
//
// Copyright (c) 2023 Georgiy Lebedev, Dmitrii Ustiugov, Plamen Petrov and vHive team
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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/vhive-serverless/vhive/snapshotting"

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

	"github.com/go-multierror/multierror"
	"github.com/vhive-serverless/vhive/memory/manager"
	"github.com/vhive-serverless/vhive/metrics"
	"github.com/vhive-serverless/vhive/misc"

	_ "github.com/davecgh/go-spew/spew" //tmp
)

// StartVMResponse is the response returned by StartVM
// TODO: Integrate response with non-cri API
type StartVMResponse struct {
	// VMID is the effective VM ID. It differs from the requested ID when a
	// pre-created shim was acquired from the shim pool.
	VMID string
	// GuestIP is the IP of the guest MicroVM
	GuestIP string
}

const (
	testImageName        = "ghcr.io/ease-lab/helloworld:var_workload"
	baseSnapshotRevision = "vhive-base-snapshot"
)

func withNamespace(ctx context.Context, snapshotter, vmID string) context.Context {
	if snapshotter == "proxy" {
		// http-address-resolver assumes that the containerd namespace is the VM ID
		// https://github.com/firecracker-microvm/firecracker-containerd/tree/main/snapshotter#address-resolver-agent
		return namespaces.WithNamespace(ctx, vmID)
	}
	return namespaces.WithNamespace(ctx, namespaceName)
}

// StartVM Boots a VM if it does not exist
func (o *Orchestrator) StartVM(ctx context.Context, vmID, imageName string) (_ *StartVMResponse, _ *metrics.Metric, retErr error) {
	return o.StartVMWithEnvironment(ctx, vmID, imageName, []string{})
}

func (o *Orchestrator) StartVMWithEnvironment(ctx context.Context, vmID, imageName string, environmentVariables []string) (_ *StartVMResponse, _ *metrics.Metric, retErr error) {
	if err := o.validateBaseSnapshotMode(); err != nil {
		return nil, nil, err
	}
	if o.baseSnapshotEnabled {
		return o.startVMFromBaseSnapshot(ctx, vmID, imageName, environmentVariables)
	}

	var (
		startVMMetric = metrics.NewMetric()
		tStart        time.Time
	)

	pooledShim := vmID == ""
	if pooledShim {
		var err error
		vmID, err = o.AcquireShimFromPool(ctx)
		if err != nil {
			return nil, nil, err
		}
	}

	logger := log.WithFields(log.Fields{"vmID": vmID, "image": imageName})
	logger.Debug("StartVM: Received StartVM")
	defer func() {
		if retErr != nil && pooledShim {
			if err := o.DiscardShim(ctx, vmID); err != nil {
				logger.WithError(err).Warn("failed to discard shim after VM launch failure")
			}
		}
	}()

	vm, err := o.vmPool.Allocate(vmID)
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

	ctx = withNamespace(ctx, o.snapshotter, vmID)

	// With remote snapshotters, we first create the VM and then pull the image, since the snapshotter lives inside the VM
	if o.snapshotter != "proxy" {
		tStart = time.Now()
		if vm.Image, err = o.getImage(ctx, imageName); err != nil {
			return nil, nil, errors.Wrapf(err, "Failed to get/pull image")
		}
		o.registerImageProvenance(ctx, imageName, *vm.Image)
		startVMMetric.MetricMap[metrics.GetImage] = metrics.ToUS(time.Since(tStart))
	}

	tStart = time.Now()
	conf := o.getVMConfig(vm)
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

	if o.snapshotter == "proxy" {
		tStart = time.Now()
		if _, err = o.fcClient.SetVMMetadata(ctx, &proto.SetVMMetadataRequest{
			VMID:     vmID,
			Metadata: o.GetDockerCredentials(),
		}); err != nil {
			logger.WithError(err).Error("failed to set VM metadata")
			return nil, nil, errors.Wrap(err, "failed to set VM metadata")
		}

		if vm.Image, err = o.getImage(ctx, imageName); err != nil {
			return nil, nil, errors.Wrapf(err, "Failed to get/pull image")
		}
		o.registerImageProvenance(ctx, imageName, *vm.Image)
		startVMMetric.MetricMap[metrics.GetImage] = metrics.ToUS(time.Since(tStart))
	}

	if err := o.startContainerTask(ctx, vm, environmentVariables, startVMMetric, int(conf.MachineCfg.MemSizeMib)); err != nil {
		return nil, nil, err
	}

	logger.Debug("Successfully started a VM")

	return &StartVMResponse{VMID: vmID, GuestIP: vm.GetIP()}, startVMMetric, nil
}

// startVMFromBaseSnapshot restores an image-less VM and only then pulls the
// requested image. Remote snapshotters require that ordering because image
// resolution happens in the guest.
func (o *Orchestrator) startVMFromBaseSnapshot(ctx context.Context, vmID, imageName string, environmentVariables []string) (_ *StartVMResponse, _ *metrics.Metric, retErr error) {
	pooledShim := vmID == ""
	defer func() {
		if retErr != nil && pooledShim && vmID != "" {
			if err := o.DiscardShim(ctx, vmID); err != nil {
				log.WithError(err).WithField("vmID", vmID).Warn("failed to discard shim after base snapshot launch failure")
			}
		}
	}()

	if err := o.ensureBaseSnapshot(ctx); err != nil {
		return nil, nil, err
	}
	snap, err := o.baseSnapshotManager.AcquireSnapshotContext(ctx, baseSnapshotRevision)
	if err != nil {
		return nil, nil, errors.Wrap(err, "acquire base snapshot")
	}

	resp, metric, err := o.LoadSnapshot(ctx, vmID, snap)
	if err != nil {
		return nil, nil, errors.Wrap(err, "restore base snapshot")
	}
	vmID = resp.VMID
	ctx = withNamespace(ctx, o.snapshotter, vmID)
	if _, err := o.fcClient.SetVMMetadata(ctx, &proto.SetVMMetadataRequest{VMID: vmID, Metadata: o.GetDockerCredentials()}); err != nil {
		_ = o.StopSingleVM(ctx, vmID)
		return nil, nil, errors.Wrap(err, "set VM metadata after base restore")
	}
	resumeMetric, err := o.ResumeVM(ctx, vmID)
	if err != nil {
		_ = o.StopSingleVM(ctx, vmID)
		return nil, nil, errors.Wrap(err, "resume base snapshot")
	}
	for name, value := range resumeMetric.MetricMap {
		metric.MetricMap[name] = value
	}

	vm, err := o.vmPool.GetVM(vmID)
	if err != nil {
		_ = o.StopSingleVM(ctx, vmID)
		return nil, nil, err
	}
	tStart := time.Now()
	if vm.Image, err = o.getImage(ctx, imageName); err != nil {
		_ = o.StopSingleVM(ctx, vmID)
		return nil, nil, errors.Wrapf(err, "get/pull image after base restore")
	}
	o.registerImageProvenance(ctx, imageName, *vm.Image)
	metric.MetricMap[metrics.GetImage] = metrics.ToUS(time.Since(tStart))
	if err := o.startContainerTask(ctx, vm, environmentVariables, metric, int(o.getVMConfig(vm).MachineCfg.MemSizeMib)); err != nil {
		_ = o.StopSingleVM(ctx, vmID)
		return nil, nil, err
	}
	return &StartVMResponse{VMID: vmID, GuestIP: vm.GetIP()}, metric, nil
}

// ensureBaseSnapshot records and publishes exactly one image-less VM snapshot.
// It deliberately never registers the VM with the memory manager, so no
// working set is captured or published for this shared base.
func (o *Orchestrator) ensureBaseSnapshot(ctx context.Context) error {
	o.baseSnapshotOnce.Do(func() {
		if _, err := o.baseSnapshotManager.AcquireSnapshotContext(ctx, baseSnapshotRevision); err == nil {
			return
		} else if !errors.Is(err, snapshotting.ErrSnapshotNotFound) && !errors.Is(err, snapshotting.ErrArtifactNotFound) {
			o.baseSnapshotErr = err
			return
		}

		vmID := baseSnapshotRevision
		vm, err := o.vmPool.Allocate(vmID)
		if err != nil {
			o.baseSnapshotErr = err
			return
		}
		baseCtx := withNamespace(ctx, o.snapshotter, vmID)
		defer func() { _ = o.vmPool.Free(vmID) }()
		defer func() { _, _ = o.fcClient.StopVM(baseCtx, &proto.StopVMRequest{VMID: vmID, TimeoutSeconds: 1}) }()

		if _, err = o.fcClient.CreateVM(baseCtx, o.getVMConfig(vm)); err != nil {
			o.baseSnapshotErr = errors.Wrap(err, "create image-less base VM")
			return
		}
		if err = o.PauseVM(baseCtx, vmID); err != nil {
			o.baseSnapshotErr = errors.Wrap(err, "pause image-less base VM")
			return
		}
		var snap *snapshotting.Snapshot
		snap, err = o.baseSnapshotManager.InitSnapshot(baseSnapshotRevision, "")
		if err == nil {
			err = o.CreateSnapshot(baseCtx, vmID, snap)
		}
		if err == nil && o.contentProvenance != nil {
			err = o.contentProvenance.AddBaseRootfsFiles(uint64(os.Getpagesize()), snap.GetMemFilePath())
		}
		if err == nil {
			err = o.baseSnapshotManager.CommitSnapshot(baseSnapshotRevision)
		}
		if err == nil {
			err = o.baseSnapshotManager.PublishSnapshot(baseCtx, baseSnapshotRevision)
		}
		if err != nil {
			o.baseSnapshotErr = errors.Wrap(err, "create base snapshot")
		}
	})
	return o.baseSnapshotErr
}

// startContainerTask attaches an image-specific container and task to an
// already booted VM. Fresh boots and restored base snapshots share this exact
// path; only the VM boot operation differs.
func (o *Orchestrator) startContainerTask(ctx context.Context, vm *misc.VM, environmentVariables []string, metric *metrics.Metric, memSizeMiB int) (retErr error) {
	logger := log.WithField("vmID", vm.ID)
	specOpts := []oci.SpecOpts{
		oci.WithEnv(environmentVariables),
		firecrackeroci.WithVMID(vm.ID),
		firecrackeroci.WithVMNetwork,
	}
	if o.snapshotter == "proxy" {
		// Remote snapshotters keep the image filesystem in the guest, so the
		// agent must resolve image user/group settings there.
		specOpts = append(specOpts, firecrackeroci.WithVMLocalImageConfig(*vm.Image))
	} else {
		specOpts = append(specOpts, oci.WithImageConfig(*vm.Image))
	}
	tStart := time.Now()
	container, err := o.client.NewContainer(
		ctx,
		vm.ContainerSnapKey,
		containerd.WithSnapshotter(o.snapshotter),
		containerd.WithNewSnapshot(vm.ContainerSnapKey, *vm.Image),
		containerd.WithNewSpec(specOpts...),
		containerd.WithRuntime("aws.firecracker", nil),
	)
	metric.MetricMap[metrics.NewContainer] = metrics.ToUS(time.Since(tStart))
	vm.Container = &container
	if err != nil {
		return errors.Wrap(err, "failed to create a container")
	}
	defer func() {
		if retErr != nil {
			if err := container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
				logger.WithError(err).Error("failed to delete container after failure")
			}
		}
	}()

	iologger := NewWorkloadIoWriter(vm.ID)
	o.workloadIo.Store(vm.ID, &iologger)
	defer func() {
		if retErr != nil {
			o.workloadIo.Delete(vm.ID)
		}
	}()
	logger.Debug("StartVM: Creating a new task")
	tStart = time.Now()
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStreams(os.Stdin, iologger, iologger)))
	metric.MetricMap[metrics.NewTask] = metrics.ToUS(time.Since(tStart))
	vm.Task = &task
	if err != nil {
		return errors.Wrap(err, "failed to create a task")
	}
	defer func() {
		if retErr != nil {
			if _, err := task.Delete(ctx); err != nil {
				logger.WithError(err).Debug("failed to delete task after launch failure")
			}
		}
	}()
	logger.Debug("StartVM: Waiting for the task to get ready")
	tStart = time.Now()
	ch, err := task.Wait(ctx)
	metric.MetricMap[metrics.TaskWait] = metrics.ToUS(time.Since(tStart))
	vm.TaskCh = ch
	if err != nil {
		return errors.Wrap(err, "failed to wait for a task")
	}
	defer func() {
		if retErr != nil {
			if err := task.Kill(ctx, syscall.SIGKILL); err != nil {
				logger.WithError(err).Debug("failed to kill task after launch failure")
			}
		}
	}()
	logger.Debug("StartVM: Starting the task")
	tStart = time.Now()
	if err := task.Start(ctx); err != nil {
		return errors.Wrap(err, "failed to start a task")
	}
	metric.MetricMap[metrics.TaskStart] = metrics.ToUS(time.Since(tStart))
	defer func() {
		if retErr != nil {
			if err := task.Kill(ctx, syscall.SIGKILL); err != nil {
				logger.WithError(err).Debug("failed to kill task after launch failure")
			}
		}
	}()

	if err := os.MkdirAll(o.getVMBaseDir(vm.ID), 0777); err != nil {
		return err
	}
	if o.GetUPFEnabled() {
		logger.Debug("Registering VM with the memory manager")
		if err := o.memoryManager.RegisterVM(manager.SnapshotStateCfg{
			VMID:           vm.ID,
			GuestMemPath:   o.getMemoryFile(vm.ID),
			BaseDir:        o.getVMBaseDir(vm.ID),
			GuestMemSize:   memSizeMiB * 1024 * 1024,
			IsLazyMode:     o.isLazyMode,
			WSCoalescing:   o.wsCoalescing,
			VMMStatePath:   o.getSnapshotFile(vm.ID),
			WorkingSetPath: o.getWorkingSetFile(vm.ID),
		}); err != nil {
			return errors.Wrap(err, "register VM with memory manager")
		}
	}
	return nil
}

// StopSingleVM Shuts down a VM
// Note: VMs are not quisced before being stopped
func (o *Orchestrator) StopSingleVM(ctx context.Context, vmID string) error {
	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Orchestrator received StopVM")

	ctx = withNamespace(ctx, o.snapshotter, vmID)
	vm, err := o.vmPool.GetVM(vmID)
	if err != nil {
		if _, ok := err.(*misc.NonExistErr); ok {
			logger.Panic("StopVM: VM does not exist")
		}
		logger.Panic("StopVM: GetVM() failed for an unknown reason")

	}

	logger = log.WithFields(log.Fields{"vmID": vmID})

	// FIXME (gh-818)
	//if !vm.SnapBooted {
	//	time.Sleep(3 * time.Second)
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

	if o.GetUPFEnabled() {
		if err := o.memoryManager.MarkTerminating(vmID); err != nil {
			logger.WithError(err).Warn("failed to mark VM as terminating in memory manager")
		}
	}

	if _, err := o.fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID, TimeoutSeconds: 1}); err != nil {
		logger.WithError(err).Error("failed to stop firecracker-containerd VM")
	}

	if o.GetUPFEnabled() {
		if err := o.memoryManager.Deactivate(vmID); err != nil {
			logger.WithError(err).Warn("failed to deactivate VM in memory manager")
		}
	}

	if err := o.vmPool.Free(vmID); err != nil {
		logger.Error("failed to free VM from VM pool")
		return err
	}

	o.workloadIo.Delete(vmID)

	if vm.SnapBooted && o.snapshotter == "devmapper" {
		if err := o.devMapper.RemoveDeviceSnapshot(ctx, vm.ContainerSnapKey); err != nil {
			logger.Error("failed to deactivate container snapshot")
			return err
		}
	}

	logger.Debug("Stopped VM successfully")

	return nil
}

func (o *Orchestrator) getImage(ctx context.Context, imageName string) (*containerd.Image, error) {
	// Images cannot be marked as cached if using remote snapshotters because they are pulled inside the VM
	return o.imageManager.GetImage(ctx, imageName, o.snapshotter != "proxy")
}

// registerImageProvenance adds page hashes from the immutable devmapper image
// parent before any container-specific writable snapshot is created. A failed
// lookup is conservative: those pages stay revision-private.
func (o *Orchestrator) registerImageProvenance(ctx context.Context, imageName string, imageRef containerd.Image) {
	if o.contentProvenance == nil {
		return
	}
	if o.snapshotter == "devmapper" {
		snapshot, err := o.devMapper.GetImageSnapshot(ctx, imageRef)
		if err != nil {
			log.WithError(err).WithField("image", imageName).Warn("image provenance unavailable; image pages remain private")
			return
		}
		if err := o.contentProvenance.AddImageFiles(uint64(os.Getpagesize()), imageName, snapshot.GetDevicePath()); err != nil {
			log.WithError(err).WithField("image", imageName).Warn("failed to index image provenance; image pages remain private")
		}
		return
	}
	path, ok := provenanceImageSourcePath(o.ProvenanceImageSourceDir(), imageName)
	if !ok {
		log.WithField("image", imageName).Warn("stargz image provenance source unavailable; image pages remain private")
		return
	}
	if err := o.contentProvenance.AddImageFiles(uint64(os.Getpagesize()), imageName, path); err != nil {
		log.WithError(err).WithField("image", imageName).Warn("failed to index stargz image provenance; image pages remain private")
	}
}

func provenanceImageSourcePath(directory, image string) (string, bool) {
	if directory == "" {
		return "", false
	}
	data, err := os.ReadFile(filepath.Join(directory, "index.tsv"))
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) == 2 && parts[0] == image {
			return parts[1], true
		}
	}
	return "", false
}

func (o *Orchestrator) getVMConfig(vm *misc.VM) *proto.CreateVMRequest {
	kernelArgs := "ro noapic reboot=k panic=1 acpi=off pci=off nomodules systemd.log_color=false systemd.journald.forward_to_console systemd.unit=firecracker.target init=/sbin/overlay-init tsc=reliable quiet ipv6.disable=1 console=ttyS0"

	return &proto.CreateVMRequest{
		VMID:           vm.ID,
		TimeoutSeconds: 100,
		KernelArgs:     kernelArgs,
		MachineCfg: &proto.FirecrackerMachineConfiguration{
			VcpuCount:  1,
			MemSizeMib: 512,
		},
		NetworkInterfaces: []*proto.FirecrackerNetworkInterface{{
			AllowMMDS: true,
			StaticConfig: &proto.StaticNetworkConfiguration{
				MacAddress:  vm.GetMacAddress(),
				HostDevName: vm.GetHostDevName(),
				IPConfig: &proto.IPConfiguration{
					PrimaryAddr: vm.GetPrimaryAddr(),
					GatewayAddr: vm.GetGatewayAddr(),
					Nameservers: o.dns,
				},
			},
		}},
		NetNS: vm.GetNetworkNamespace(),
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

	ctx = withNamespace(ctx, o.snapshotter, vmID)

	if _, err := o.fcClient.PauseVM(ctx, &proto.PauseVMRequest{VMID: vmID}); err != nil {
		logger.WithError(err).Error("failed to pause the VM")
		return err
	}

	return nil
}

// ResumeVM Resumes a VM
func (o *Orchestrator) ResumeVM(ctx context.Context, vmID string) (*metrics.Metric, error) {
	var (
		resumeVMMetric = metrics.NewMetric()
		tStart         time.Time
	)

	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Orchestrator received ResumeVM")

	ctx = withNamespace(ctx, o.snapshotter, vmID)

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

	ctx = withNamespace(ctx, o.snapshotter, vmID)

	req := &proto.CreateSnapshotRequest{
		VMID:         vmID,
		SnapshotPath: snap.GetSnapshotFilePath(),
		MemFilePath:  snap.GetMemFilePath(),
	}

	if _, err := o.fcClient.CreateSnapshot(ctx, req); err != nil {
		logger.WithError(err).Error("failed to create snapshot of the VM")
		return err
	}

	vm, err := o.vmPool.GetVM(vmID)
	if err != nil {
		return err
	}

	if o.snapshotter == "devmapper" {
		patchFilePath := snap.GetPatchFilePath()
		logger = log.WithFields(log.Fields{"vmID": vmID, "patchFilePath": patchFilePath})
		logger.Debug("Creating patch file with disk state difference")
		if err := o.devMapper.CreatePatch(ctx, patchFilePath, vm.ContainerSnapKey, *vm.Image); err != nil {
			logger.WithError(err).Error("failed to create container patch file")
			return err
		}
	}

	logger = log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Serializing snapshot info")
	if err := snap.SerializeSnapInfo(); err != nil {
		logger.WithError(err).Error("failed to serialize snapshot info")
		return err
	}

	return nil
}

func logSnapshotLoadFailure(logger *log.Entry, snap *snapshotting.Snapshot, conf *proto.CreateVMRequest, err error) {
	logger.WithError(err).WithFields(log.Fields{
		"snapFilePath":          snap.GetSnapshotFilePath(),
		"memFilePath":           snap.GetMemFilePath(),
		"containerSnapshotPath": conf.ContainerSnapshotPath,
		"memoryBackendType":     conf.GetMemBackend().GetBackendType(),
		"memoryBackendPath":     conf.GetMemBackend().GetBackendPath(),
	}).Error("failed to load snapshot of the VM")
}

func configureSnapshotMemoryBackend(conf *proto.CreateVMRequest, backendType, backendPath string) {
	conf.MemFilePath = ""
	conf.MemBackend = &proto.MemoryBackend{
		BackendType: backendType,
		BackendPath: backendPath,
	}
}

var newRecipePageSourceForRevision = snapshotting.NewRecipePageSourceForRevision
var newProvenanceWorkingSetPageSource = snapshotting.NewProvenanceWorkingSetPageSource

// recipePageServer supplies chunked memory through UFFD only when the
// SnapshotManager did not materialize a complete local memory file. Lazy mode
// itself is exclusively the working-set-trace replay policy.
func recipePageServer(ctx context.Context, store snapshotting.ArtifactStore, snap *snapshotting.Snapshot) (*manager.PageServer, error) {
	if snap == nil || !snap.HasMemoryRecipe() {
		return nil, nil
	}
	if _, err := os.Stat(snap.GetMemFilePath()); err == nil {
		return nil, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat reconstructed memory file: %w", err)
	}
	if store == nil {
		return nil, fmt.Errorf("recipe-backed restore requires an artifact store")
	}
	source, err := newRecipePageSourceForRevision(ctx, store, nil, snap.GetId())
	if err != nil {
		return nil, err
	}
	// A missing manifest is the normal compatibility case for snapshots
	// published before provenance-aware working sets were enabled.
	if provenanceSource, provenanceErr := newProvenanceWorkingSetPageSource(ctx, store, snap.GetId(), source); provenanceErr == nil {
		source = provenanceSource
	} else if !errors.Is(provenanceErr, snapshotting.ErrArtifactNotFound) {
		_ = source.Close()
		return nil, fmt.Errorf("load provenance working set: %w", provenanceErr)
	}
	input, err := (manager.RestoreMaterializer{}).MaterializeLazy(source)
	if err != nil {
		_ = source.Close()
		return nil, err
	}
	server, err := input.NewPageServer()
	if err != nil {
		_ = input.Close()
		return nil, err
	}
	return server, nil
}

// LoadSnapshot Loads a snapshot of a VM
func (o *Orchestrator) LoadSnapshot(ctx context.Context, vmID string, snap *snapshotting.Snapshot) (_ *StartVMResponse, _ *metrics.Metric, retErr error) {
	var (
		loadSnapshotMetric = metrics.NewMetric()
		tStart             time.Time
		loadErr            error
		activateErr        error
		deactivateErr      error
		activateErrChan    chan error
		lazyPageServer     *manager.PageServer
	)

	pooledShim := vmID == ""
	if pooledShim {
		var err error
		vmID, err = o.AcquireShimFromPool(ctx)
		if err != nil {
			return nil, nil, err
		}
	}

	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Orchestrator received LoadSnapshot")
	defer func() {
		if retErr != nil && pooledShim {
			if err := o.DiscardShim(ctx, vmID); err != nil {
				logger.WithError(err).Warn("failed to discard shim after snapshot-load failure")
			}
		}
	}()

	ctx = withNamespace(ctx, o.snapshotter, vmID)

	vm, err := o.vmPool.Allocate(vmID)
	if err != nil {
		logger.Error("failed to allocate VM in VM pool")
		return nil, nil, err
	}

	defer func() {
		if retErr != nil {
			if err := o.vmPool.Free(vmID); err != nil {
				logger.WithError(err).Errorf("failed to free VM from pool after failure")
			}
		}
	}()

	conf := o.getVMConfig(vm)
	conf.LoadSnapshot = true
	conf.SnapshotPath = snap.GetSnapshotFilePath()
	uffdSock := filepath.Join(o.getVMBaseDir(vmID), "uffd.sock")
	configureSnapshotMemoryBackend(conf, "File", snap.GetMemFilePath())
	// The shared base must not acquire a working set. Its restored VM is
	// registered afresh for the image-specific workload below instead.
	enableUPF := o.GetUPFEnabled() && snap.GetId() != baseSnapshotRevision
	if enableUPF {
		lazyPageServer, err = recipePageServer(ctx, o.artifactStore, snap)
		if err != nil {
			return nil, nil, errors.Wrap(err, "prepare recipe-backed restore")
		}
	}
	defer func() {
		if retErr != nil && lazyPageServer != nil {
			_ = lazyPageServer.Close()
		}
	}()

	if o.snapshotter == "devmapper" {
		if vm.Image, err = o.getImage(ctx, snap.GetImage()); err != nil {
			return nil, nil, errors.Wrapf(err, "Failed to get/pull image")
		}

		if err := o.devMapper.CreateDeviceSnapshotFromImage(ctx, vm.ContainerSnapKey, *vm.Image); err != nil {
			return nil, nil, errors.Wrapf(err, "creating container snapshot")
		}

		containerSnap, err := o.devMapper.GetDeviceSnapshot(ctx, vm.ContainerSnapKey)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "previously created container device does not exist")
		}

		if err := o.devMapper.RestorePatch(ctx, vm.ContainerSnapKey, snap.GetPatchFilePath()); err != nil {
			return nil, nil, errors.Wrapf(err, "unpacking patch into container snapshot")
		}

		conf.ContainerSnapshotPath = containerSnap.GetDevicePath()
	} else {
		// Create default stub drive: https://github.com/vhive-serverless/firecracker-containerd/blob/master/runtime/drive_handler.go#L58
		// Default path: /var/lib/firecracker-containerd/shim-base/<namespace>#<vmID>/ctrstub0 (https://github.com/vhive-serverless/firecracker-containerd/blob/master/internal/vm/dir.go#L52)
		namespace, ok := namespaces.Namespace(ctx)
		if !ok {
			namespace = vmID // use vmID as default namespace
		}
		stubPath := filepath.Join(
			"/var/lib/firecracker-containerd/shim-base",
			fmt.Sprintf("%s#%s", namespace, vmID),
			"ctrstub0",
		)

		if _, err := os.Stat(stubPath); os.IsNotExist(err) {
			// Create default stub drive
			if err := os.MkdirAll(filepath.Dir(stubPath), 0755); err != nil {
				return nil, nil, errors.Wrapf(err, "creating stub directory")
			}

			f, err := os.OpenFile(stubPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return nil, nil, err
			}
			defer func() {
				if err := f.Close(); err != nil {
					logger.WithError(err).Errorf("unexpected error during %v close", f.Name())
				}
			}()

			// Content is the stub drive ID (base32 encoded hash of "ctrstub0" = "MN2HE43UOVRDA")
			if _, err := f.WriteString("MN2HE43UOVRDA"); err != nil {
				return nil, nil, err
			}
		}
		conf.ContainerSnapshotPath = stubPath
	}

	if enableUPF {
		configureSnapshotMemoryBackend(conf, "Uffd", uffdSock)

		if err := o.memoryManager.PrepareSnapshotLoad(manager.SnapshotStateCfg{
			VMID:                vmID,
			VMMStatePath:        snap.GetSnapshotFilePath(),
			GuestMemPath:        snap.GetMemFilePath(),
			InstanceSockAddr:    uffdSock,
			BaseDir:             o.getVMBaseDir(vmID),
			GuestMemSize:        int(conf.MachineCfg.MemSizeMib) * 1024 * 1024,
			IsLazyMode:          o.isLazyMode,
			WSCoalescing:        o.wsCoalescing,
			WorkingSetPath:      snap.GetWorkingSetFilePath(),
			WorkingSetTracePath: snap.GetWorkingSetTraceFilePath(),
			PageServer:          lazyPageServer,
		}); err != nil {
			return nil, nil, err
		}

		if err := o.memoryManager.FetchState(vmID); err != nil {
			return nil, nil, err
		}
	}

	tStart = time.Now()

	if enableUPF {
		activateErrChan = make(chan error, 1)
		socketReadyChan := make(chan struct{}, 1)
		go func() {
			err := o.memoryManager.Activate(vmID, socketReadyChan)
			if err != nil {
				logger.WithError(err).Warn("Failed to activate VM in the memory manager")
			}
			activateErrChan <- err
		}()

		select {
		case <-socketReadyChan:
		case activateErr = <-activateErrChan:
			return nil, nil, activateErr
		}
	}

	if _, loadErr = o.fcClient.CreateVM(ctx, conf); loadErr != nil {
		logSnapshotLoadFailure(logger, snap, conf, loadErr)
	}

	if activateErrChan != nil {
		activateErr = <-activateErrChan
	}

	if loadErr != nil && activateErr == nil && activateErrChan != nil {
		deactivateErr = o.memoryManager.Deactivate(vmID)
		if deactivateErr != nil {
			logger.WithError(deactivateErr).Warn("Failed to deactivate VM in the memory manager after snapshot load failure")
		}
	}

	loadSnapshotMetric.MetricMap[metrics.LoadVMM] = metrics.ToUS(time.Since(tStart))

	if loadErr != nil || activateErr != nil || deactivateErr != nil {
		multierr := multierror.Of(loadErr, activateErr, deactivateErr)
		return nil, nil, multierr
	}

	vm.SnapBooted = true

	return &StartVMResponse{VMID: vmID, GuestIP: vm.GetIP()}, loadSnapshotMetric, nil
}
