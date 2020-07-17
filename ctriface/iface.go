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
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"

	fcclient "github.com/firecracker-microvm/firecracker-containerd/firecracker-control/client"
	"github.com/firecracker-microvm/firecracker-containerd/proto" // note: from the original repo
	"github.com/firecracker-microvm/firecracker-containerd/runtime/firecrackeroci"
	"github.com/pkg/errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	_ "google.golang.org/grpc/codes"  //tmp
	_ "google.golang.org/grpc/status" //tmp

	hpb "github.com/ustiugov/fccd-orchestrator/helloworld"
	"github.com/ustiugov/fccd-orchestrator/misc"

	_ "github.com/davecgh/go-spew/spew" //tmp
)

const (
	containerdAddress      = "/run/firecracker-containerd/containerd.sock"
	containerdTTRPCAddress = containerdAddress + ".ttrpc"
	namespaceName          = "firecracker-containerd"
)

// Orchestrator Drives all VMs
type Orchestrator struct {
	niNum        int
	vmPool       *misc.VMPool
	cachedImages map[string]containerd.Image
	snapshotter  string
	client       *containerd.Client
	fcClient     *fcclient.Client
	// store *skv.KVStore
	snapshotsEnabled bool
	isUPFEnabled     bool
}

// NewOrchestrator Initializes a new orchestrator
func NewOrchestrator(snapshotter string, niNum int, opts ...OrchestratorOption) *Orchestrator {
	var err error

	o := new(Orchestrator)
	o.niNum = niNum
	o.vmPool = misc.NewVMPool(o.niNum)
	o.cachedImages = make(map[string]containerd.Image)
	o.snapshotter = snapshotter
	o.snapshotsEnabled = false
	o.isUPFEnabled = false

	for _, opt := range opts {
		opt(o)
	}

	log.Info("Creating containerd client")
	o.client, err = containerd.New(containerdAddress)
	if err != nil {
		log.Fatal("Failed to start containerd client", err)
	}
	log.Info("Created containerd client")

	log.Info("Creating firecracker client")
	o.fcClient, err = fcclient.New(containerdTTRPCAddress)
	if err != nil {
		log.Fatal("Failed to start firecracker client", err)
	}
	log.Info("Created firecracker client")
	return o
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
		VMID:       vm.ID,
		KernelArgs: kernelArgs,
		MachineCfg: &proto.FirecrackerMachineConfiguration{
			VcpuCount:  1,
			MemSizeMib: 512,
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

func (o *Orchestrator) getFuncClient(ctx context.Context, vm *misc.VM, logger *logrus.Entry) (hpb.GreeterClient, error) {
	logger.Debug("getFuncClient: Calling function's gRPC server")

	backoffConfig := backoff.DefaultConfig
	backoffConfig.MaxDelay = 3 * time.Second
	connParams := grpc.ConnectParams{
		Backoff: backoffConfig,
	}

	gopts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithInsecure(),
		grpc.FailOnNonTempDialError(true),
		grpc.WithConnectParams(connParams),
		grpc.WithContextDialer(contextDialer),
	}
	ctxx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctxx, vm.Ni.PrimaryAddress+":50051", gopts...)
	vm.Conn = conn
	if err != nil {
		if errCleanup := o.cleanup(ctx, vm, true, true, true, true); errCleanup != nil {
			logger.Warn("Cleanup failed: ", errCleanup)
		}
		return nil, err
	}
	logger.Debug("getFuncClient: Creating a new gRPC client")
	return hpb.NewGreeterClient(conn), nil
}

// StartVM Boots a VM if it does not exist
func (o *Orchestrator) StartVM(ctx context.Context, vmID, imageName string) (string, string, error) {
	var tProfile string
	var tStart, tElapsed time.Time
	logger := log.WithFields(log.Fields{"vmID": vmID, "image": imageName})
	logger.Debug("StartVM: Received StartVM")

	// FIXME: does not account for Deactivating
	vm, err := o.vmPool.Allocate(vmID)
	if err != nil {
		logger.Panic("StartVM: Unknown error")
		return "StartVM: Unknown error", tProfile, err
	}

	ctx = namespaces.WithNamespace(ctx, namespaceName)
	tStart = time.Now()
	if vm.Image, err = o.getImage(ctx, imageName); err != nil {
		return "Failed to start VM", tProfile, errors.Wrapf(err, "Failed to get/pull image")
	}
	tElapsed = time.Now()
	tProfile += strconv.FormatInt(tElapsed.Sub(tStart).Microseconds(), 10) + ";"

	logger.Debug("StartVM: Creating a new VM")
	tStart = time.Now()
	_, err = o.fcClient.CreateVM(ctx, o.getVMConfig(vm))
	tElapsed = time.Now()
	tProfile += strconv.FormatInt(tElapsed.Sub(tStart).Microseconds(), 10) + ";"
	if err != nil {
		if errCleanup := o.cleanup(ctx, vm, false, false, false, false); errCleanup != nil {
			logger.Warn("Cleanup failed: ", errCleanup)
		}
		return "Failed to start VM", tProfile, errors.Wrap(err, "failed to create the VM")
	}

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
	vm.Container = &container
	tElapsed = time.Now()
	tProfile += strconv.FormatInt(tElapsed.Sub(tStart).Microseconds(), 10) + ";"
	if err != nil {
		if errCleanup := o.cleanup(ctx, vm, true, false, false, false); errCleanup != nil {
			logger.Warn("Cleanup failed: ", errCleanup)
		}
		return "Failed to start VM", tProfile, errors.Wrap(err, "failed to create a container")
	}

	logger.Debug("StartVM: Creating a new task")
	tStart = time.Now()
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	vm.Task = &task
	tElapsed = time.Now()
	tProfile += strconv.FormatInt(tElapsed.Sub(tStart).Microseconds(), 10) + ";"
	if err != nil {
		if errCleanup := o.cleanup(ctx, vm, true, true, false, false); errCleanup != nil {
			logger.Warn("Cleanup failed: ", errCleanup)
		}
		return "Failed to start VM", tProfile, errors.Wrap(err, "failed to create a task")
	}

	logger.Debug("StartVM: Waiting for the task to get ready")
	tStart = time.Now()
	ch, err := task.Wait(ctx)
	vm.TaskCh = ch
	tElapsed = time.Now()
	tProfile += strconv.FormatInt(tElapsed.Sub(tStart).Microseconds(), 10) + ";"
	if err != nil {
		if errCleanup := o.cleanup(ctx, vm, true, true, true, false); errCleanup != nil {
			logger.Warn("Cleanup failed: ", errCleanup)
		}
		return "Failed to start VM", tProfile, errors.Wrap(err, "failed to wait for a task")
	}

	logger.Debug("StartVM: Starting the task")
	tStart = time.Now()
	if err := task.Start(ctx); err != nil {
		if errCleanup := o.cleanup(ctx, vm, true, true, true, false); errCleanup != nil {
			logger.Warn("Cleanup failed: ", errCleanup)
		}
		return "Failed to start VM", tProfile, errors.Wrap(err, "failed to start a task")
	}
	tElapsed = time.Now()
	tProfile += strconv.FormatInt(tElapsed.Sub(tStart).Microseconds(), 10) + ";"

	funcClient, err := o.getFuncClient(ctx, vm, logger)
	if err != nil {
		return "Failed to start VM", tProfile, errors.Wrap(err, "failed to connect to a function")
	}
	vm.FuncClient = &funcClient

	logger.Debug("Successfully started a VM")

	return "VM, container, and task started successfully", tProfile, nil
}

// GetFuncClient Returns the client for the function
func (o *Orchestrator) GetFuncClient(vmID string) (*hpb.GreeterClient, error) {
	return o.vmPool.GetFuncClient(vmID)
}

func (o *Orchestrator) cleanup(ctx context.Context, vm *misc.VM, isVM, isCont, isTaskStop, isTaskKill bool) error {
	vmID := vm.ID

	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Cleaning up after a failure")

	if isTaskKill {
		task := *vm.Task
		if err := task.Kill(ctx, syscall.SIGKILL); err != nil {
			logger.Warn("Failed to kill the task: ", err)
			return errors.Wrapf(err, "Attempt to kill the task failed.")
		}
	}
	if isTaskStop {
		task := *vm.Task
		if _, err := task.Delete(ctx); err != nil {
			logger.Warn("Failed to delete the task: ", err)
			return errors.Wrapf(err, "Attempt to delete the task failed.")
		}
	}
	if isCont {
		cont := *vm.Container
		if err := cont.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
			logger.Warn("Failed to delete the container: ", err)
			return errors.Wrapf(err, "Attempt to delete the container failed.")
		}
	}

	if isVM {
		if _, err := o.fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID}); err != nil {
			logger.Warn("Failed to stop the VM: ", err)
			return errors.Wrapf(err, "Attempt to stop the VM failed.")
		}
	}

	if err := o.vmPool.Free(vmID); err != nil {
		return err
	}

	logger.Debug("Cleaned up successfully")

	return nil
}

// StopSingleVM Shuts down a VM
// Note: VMs are not quisced before being stopped
func (o *Orchestrator) StopSingleVM(ctx context.Context, vmID string) (string, error) {
	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Orchestrator received StopVM")

	ctx = namespaces.WithNamespace(ctx, namespaceName)
	vm, err := o.vmPool.GetVM(vmID)
	if err != nil {
		if _, ok := err.(*misc.NonExistErr); ok {
			logger.Panic("StopVM: VM does not exist")
			return "VM does not exist", nil
		}
		logger.Panic("StopVM: GetVM() failed for an unknown reason")

	}

	logger = log.WithFields(log.Fields{"vmID": vmID})

	if err := vm.Conn.Close(); err != nil {
		logger.Warn("Failed to close the connection to function: ", err)
		return "Closing connection to function in VM " + vmID + " failed", err
	}

	task := *vm.Task
	if err := task.Kill(ctx, syscall.SIGKILL); err != nil {
		logger.Warn("Failed to kill the task: ", err)
		return "Killing task of VM " + vmID + " failed", err
	}
	if _, err := task.Delete(ctx); err != nil {
		logger.Warn("failed to delete the task of the VM: ", err)
		return "Deleting task of VM " + vmID + " failed", err
	}

	container := *vm.Container
	if err := container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
		logger.Warn("failed to delete the container of the VM: ", err)
		return "Deleting container of VM " + vmID + " failed", err
	}

	if _, err := o.fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID}); err != nil {
		logger.Warn("failed to stop the VM: ", err)
		return "Stopping VM " + vmID + " failed", err
	}

	if err := o.vmPool.Free(vmID); err != nil {
		return "Free", err
	}

	logger.Debug("Stopped VM successfully")

	return "VM " + vmID + " stopped successfully", nil
}

// StopActiveVMs Shuts down all active VMs
func (o *Orchestrator) StopActiveVMs() error {
	var vmGroup sync.WaitGroup
	for vmID, vm := range o.vmPool.GetVMMap() {
		vmGroup.Add(1)
		logger := log.WithFields(log.Fields{"vmID": vmID})
		go func(vmID string, vm *misc.VM) {
			defer vmGroup.Done()
			message, err := o.StopSingleVM(context.Background(), vmID)
			if err != nil {
				logger.Warn(message, err)
			}
			logger.Info(message)
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

func (o *Orchestrator) setupHeartbeat() {
	heartbeat := time.NewTicker(60 * time.Second)

	go func() {
		for {
			<-heartbeat.C
			log.Info("HEARTBEAT: number of active VMs: ", len(o.vmPool.GetVMMap()))
		} // for
	}() // go func
}

func (o *Orchestrator) setupCloseHandler() {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Info("\r- Ctrl+C pressed in Terminal")
		_ = o.StopActiveVMs()
		o.Cleanup()
		os.Exit(0)
	}()
}

// Cleanup Removes the bridges created by the VM pool's tap manager
func (o *Orchestrator) Cleanup() {
	o.vmPool.RemoveBridges()
}

// GetSnapshotsEnabled Returns the snapshots mode of the orchestrator
func (o *Orchestrator) GetSnapshotsEnabled() bool {
	return o.snapshotsEnabled
}

// GetUPFEnabled Returns the UPF mode of the orchestrator
func (o *Orchestrator) GetUPFEnabled() bool {
	return o.isUPFEnabled
}

// PauseVM Pauses a VM
func (o *Orchestrator) PauseVM(ctx context.Context, vmID string) (string, error) {
	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Orchestrator received PauseVM")

	ctx = namespaces.WithNamespace(ctx, namespaceName)

	if _, err := o.fcClient.PauseVM(ctx, &proto.PauseVMRequest{VMID: vmID}); err != nil {
		logger.Warn("failed to pause the VM: ", err)
		return "Pausing VM " + vmID + " failed", err
	}

	return "VM " + vmID + " paused successfully", nil
}

// ResumeVM Resumes a VM
func (o *Orchestrator) ResumeVM(ctx context.Context, vmID string) (string, error) {
	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Orchestrator received ResumeVM")

	ctx = namespaces.WithNamespace(ctx, namespaceName)

	if _, err := o.fcClient.ResumeVM(ctx, &proto.ResumeVMRequest{VMID: vmID}); err != nil {
		logger.Warn("failed to pause the VM: ", err)
		return "Resuming VM " + vmID + " failed", err
	}

	vm, err := o.vmPool.GetVM(vmID)
	if err != nil {
		return "Snapshot of VM " + vmID + " loaded successfully", err
	}

	funcClient, err := o.getFuncClient(ctx, vm, logger)
	if err != nil {
		return "Failed to start VM", errors.Wrap(err, "failed to connect to a function")
	}
	vm.FuncClient = &funcClient

	return "VM " + vmID + " resumed successfully", nil
}

// CreateSnapshot Creates a snapshot of a VM
func (o *Orchestrator) CreateSnapshot(ctx context.Context, vmID, snapPath, memPath string) (string, error) {
	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Orchestrator received CreateSnapshot")

	ctx = namespaces.WithNamespace(ctx, namespaceName)

	req := &proto.CreateSnapshotRequest{VMID: vmID, SnapshotFilePath: snapPath, MemFilePath: memPath}

	if _, err := o.fcClient.CreateSnapshot(ctx, req); err != nil {
		logger.Warn("failed to create snapshot of the VM: ", err)
		return "Creating snapshot of VM " + vmID + " failed", err
	}

	return "Snapshot of VM " + vmID + " created successfully", nil
}

// LoadSnapshot Loads a snapshot of a VM
func (o *Orchestrator) LoadSnapshot(ctx context.Context, vmID, snapPath, memPath string) (string, error) {
	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Orchestrator received LoadSnapshot")

	ctx = namespaces.WithNamespace(ctx, namespaceName)

	req := &proto.LoadSnapshotRequest{
		VMID:             vmID,
		SnapshotFilePath: snapPath,
		MemFilePath:      memPath,
		EnableUserPF:     o.GetUPFEnabled(),
	}

	if _, err := o.fcClient.LoadSnapshot(ctx, req); err != nil {
		logger.Warn("failed to load snapshot of the VM: ", err)
		return "Loading snapshot of VM " + vmID + " failed", err
	}

	return "Snapshot of VM " + vmID + " loaded successfully", nil
}

// Offload Shuts down the VM but leaves shim and other resources running.
func (o *Orchestrator) Offload(ctx context.Context, vmID string) (string, error) {
	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Orchestrator received Offload")

	ctx = namespaces.WithNamespace(ctx, namespaceName)

	if _, err := o.fcClient.Offload(ctx, &proto.OffloadRequest{VMID: vmID}); err != nil {
		logger.Warn("failed to offload the VM: ", err)
		return "Offloading VM " + vmID + " failed", err
	}

	return "VM " + vmID + " offloaded successfully", nil
}

// StopSingleVMOnly Shuts down a VM, but does not delete the task or container
// Note: VMs are not quisced before being stopped
// Broken as of now
func (o *Orchestrator) StopSingleVMOnly(ctx context.Context, vmID string) (string, error) {
	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Orchestrator received StopVMOnly")

	ctx = namespaces.WithNamespace(ctx, namespaceName)
	vm, err := o.vmPool.GetVM(vmID)
	if err != nil {
		if _, ok := err.(*misc.NonExistErr); ok {
			logger.Panic("StopVMOnly: VM does not exist")
			return "VM does not exist", nil
		}
		logger.Panic("StopVMOnly: GetVM() failed for an unknown reason")

	}

	logger = log.WithFields(log.Fields{"vmID": vmID})

	if err := vm.Conn.Close(); err != nil {
		logger.Warn("Failed to close the connection to function: ", err)
		return "Closing connection to function in VM " + vmID + " failed", err
	}

	if _, err := o.fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID}); err != nil {
		logger.Warn("failed to stop the VM: ", err)
		return "Stopping VM " + vmID + " failed", err
	}

	if err := o.vmPool.Free(vmID); err != nil {
		return "Free", err
	}

	logger.Debug("Stopped VM successfully")

	return "VM " + vmID + " stopped successfully", nil
}
