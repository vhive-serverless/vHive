// MIT License
//
// Copyright (c) 2020 Dmitrii Ustiugov, Plamen Petrov
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

	"github.com/go-multierror/multierror"
	"github.com/ustiugov/fccd-orchestrator/memory/manager"
	"github.com/ustiugov/fccd-orchestrator/metrics"
	"github.com/ustiugov/fccd-orchestrator/misc"

	_ "github.com/davecgh/go-spew/spew" //tmp
)

// TODO: Integrate response with non-cri API
// StartVMResponse is the reponse return by StartVM
type StartVMResponse struct {
	// GuestIP is the IP of the guest MicroVM
	GuestIP string
}

// StartVM Boots a VM if it does not exist
func (o *Orchestrator) StartVM(ctx context.Context, vmID, imageName string) (_ *StartVMResponse, _ *metrics.Metric, retErr error) {
	var (
		startVMMetric *metrics.Metric = metrics.NewMetric()
		tStart        time.Time
	)

	logger := log.WithFields(log.Fields{"vmID": vmID, "image": imageName})
	logger.Debug("StartVM: Received StartVM")

	// FIXME: does not account for Deactivating
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

	ctx = namespaces.WithNamespace(ctx, namespaceName)
	tStart = time.Now()
	if vm.Image, err = o.getImage(ctx, imageName); err != nil {
		return nil, nil, errors.Wrapf(err, "Failed to get/pull image")
	}
	startVMMetric.MetricMap[metrics.GetImage] = metrics.ToUS(time.Since(tStart))

	tStart = time.Now()
	conf := o.getVMConfig(vm)
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

	logger.Debug("StartVM: Creating a new container")
	tStart = time.Now()
	container, err := o.client.NewContainer(
		ctx,
		vmID,
		containerd.WithSnapshotter(o.snapshotter),
		containerd.WithNewSnapshot(vmID, *vm.Image),
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

	logger.Debug("StartVM: Creating a new task")
	tStart = time.Now()
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
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

	logger.Debug("Successfully started a VM")

	return &StartVMResponse{GuestIP: vm.Ni.PrimaryAddress}, startVMMetric, nil
}

// StopSingleVM Shuts down a VM
// Note: VMs are not quisced before being stopped
func (o *Orchestrator) StopSingleVM(ctx context.Context, vmID string) error {
	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Orchestrator received StopVM")

	ctx = namespaces.WithNamespace(ctx, namespaceName)
	vm, err := o.vmPool.GetVM(vmID)
	if err != nil {
		if _, ok := err.(*misc.NonExistErr); ok {
			logger.Panic("StopVM: VM does not exist")
		}
		logger.Panic("StopVM: GetVM() failed for an unknown reason")

	}

	logger = log.WithFields(log.Fields{"vmID": vmID})

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

	if _, err := o.fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID}); err != nil {
		logger.WithError(err).Error("failed to stop firecracker-containerd VM")
		return err
	}

	if err := o.vmPool.Free(vmID); err != nil {
		logger.Error("failed to free VM from VM pool")
		return err
	}

	logger.Debug("Stopped VM successfully")

	return nil
}

func (o *Orchestrator) getImage(ctx context.Context, imageName string) (*containerd.Image, error) {
	image, found := o.cachedImages[imageName]
	if !found {
		var err error
		log.Debug(fmt.Sprintf("Pulling image %s", imageName))
		image, err = o.client.Pull(ctx, "docker.io/"+imageName,
			containerd.WithPullUnpack,
			containerd.WithPullSnapshotter(o.snapshotter),
		)
		if err != nil {
			return &image, err
		}
		o.cachedImages[imageName] = image
	}

	return &image, nil
}

func (o *Orchestrator) getVMConfig(vm *misc.VM) *proto.CreateVMRequest {
	kernelArgs := "ro noapic reboot=k panic=1 pci=off nomodules systemd.log_color=false systemd.unit=firecracker.target init=/sbin/overlay-init tsc=reliable quiet 8250.nr_uarts=0 ipv6.disable=1"

	return &proto.CreateVMRequest{
		VMID:           vm.ID,
		TimeoutSeconds: 100,
		KernelArgs:     kernelArgs,
		MachineCfg: &proto.FirecrackerMachineConfiguration{
			VcpuCount:  1,
			MemSizeMib: 256,
		},
		NetworkInterfaces: []*proto.FirecrackerNetworkInterface{{
			StaticConfig: &proto.StaticNetworkConfiguration{
				MacAddress:  vm.Ni.MacAddress,
				HostDevName: vm.Ni.HostDevName,
				IPConfig: &proto.IPConfiguration{
					PrimaryAddr: vm.Ni.PrimaryAddress + vm.Ni.Subnet,
					GatewayAddr: vm.Ni.GatewayAddress,
				},
			},
		}},
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
	o.fcClient.Close()
	log.Info("Closing containerd client")
	o.client.Close()

	return nil
}

// PauseVM Pauses a VM
func (o *Orchestrator) PauseVM(ctx context.Context, vmID string) error {
	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Orchestrator received PauseVM")

	ctx = namespaces.WithNamespace(ctx, namespaceName)

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
func (o *Orchestrator) CreateSnapshot(ctx context.Context, vmID string) error {
	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Orchestrator received CreateSnapshot")

	ctx = namespaces.WithNamespace(ctx, namespaceName)

	req := &proto.CreateSnapshotRequest{
		VMID:             vmID,
		SnapshotFilePath: o.getSnapshotFile(vmID),
		MemFilePath:      o.getMemoryFile(vmID),
	}

	if _, err := o.fcClient.CreateSnapshot(ctx, req); err != nil {
		logger.WithError(err).Error("failed to create snapshot of the VM")
		return err
	}

	return nil
}

// LoadSnapshot Loads a snapshot of a VM
func (o *Orchestrator) LoadSnapshot(ctx context.Context, vmID string) (*metrics.Metric, error) {
	var (
		loadSnapshotMetric   *metrics.Metric = metrics.NewMetric()
		tStart               time.Time
		loadErr, activateErr error
		loadDone             = make(chan int)
	)

	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Orchestrator received LoadSnapshot")

	ctx = namespaces.WithNamespace(ctx, namespaceName)

	req := &proto.LoadSnapshotRequest{
		VMID:             vmID,
		SnapshotFilePath: o.getSnapshotFile(vmID),
		MemFilePath:      o.getMemoryFile(vmID),
		EnableUserPF:     o.GetUPFEnabled(),
	}

	if o.GetUPFEnabled() {
		o.memoryManager.FetchState(vmID)
	}

	tStart = time.Now()

	go func() {
		defer close(loadDone)

		if _, loadErr = o.fcClient.LoadSnapshot(ctx, req); loadErr != nil {
			logger.Error("Failed to load snapshot of the VM: ", loadErr)
		}
	}()

	if o.GetUPFEnabled() {
		if activateErr = o.memoryManager.Activate(vmID); activateErr != nil {
			logger.Warn("Failed to activate VM in the memory manager", activateErr)
		}
	}

	<-loadDone

	loadSnapshotMetric.MetricMap[metrics.Full] = metrics.ToUS(time.Since(tStart))

	if loadErr != nil || activateErr != nil {
		multierr := multierror.Of(loadErr, activateErr)
		return nil, multierr
	}

	return loadSnapshotMetric, nil
}

// Offload Shuts down the VM but leaves shim and other resources running.
func (o *Orchestrator) Offload(ctx context.Context, vmID string) error {
	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Orchestrator received Offload")

	ctx = namespaces.WithNamespace(ctx, namespaceName)

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
