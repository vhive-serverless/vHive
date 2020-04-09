// MIT License
//
// Copyright (c) 2020 Dmitrii Ustiugov
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

package ctrIface

import (
    "context"
    "log"
    "time"
    "strconv"
    "sync"
    _ "fmt"
    "os"
    "os/signal"
    "syscall"

    "github.com/containerd/containerd"
    "github.com/containerd/containerd/cio"
    "github.com/containerd/containerd/namespaces"
    "github.com/containerd/containerd/oci"

    fcclient "github.com/firecracker-microvm/firecracker-containerd/firecracker-control/client"
    "github.com/firecracker-microvm/firecracker-containerd/proto"
    "github.com/firecracker-microvm/firecracker-containerd/runtime/firecrackeroci"
    "github.com/pkg/errors"

    "google.golang.org/grpc"
    _ "google.golang.org/grpc/codes"
    _ "google.golang.org/grpc/status"

    "github.com/ustiugov/fccd-orchestrator/misc"
    hpb "github.com/ustiugov/fccd-orchestrator/helloworld"

//    "github.com/ustiugov/skv"
)

const (
    containerdAddress      = "/run/firecracker-containerd/containerd.sock"
    containerdTTRPCAddress = containerdAddress + ".ttrpc"
    namespaceName          = "firecracker-containerd"
)

type Orchestrator struct {
    vmPool *misc.VmPool
    niPool *misc.NiPool
    cachedImages map[string]containerd.Image
    snapshotter string
    client *containerd.Client
    fcClient *fcclient.Client
// store *skv.KVStore
}

func NewOrchestrator(snapshotter string, niNum int) *Orchestrator {
    var err error

    o := new(Orchestrator)
    o.vmPool = misc.NewVmPool()
    o.niPool = misc.NewNiPool(niNum*2) // overprovision NIs
    o.cachedImages= make(map[string]containerd.Image)
    o.snapshotter = snapshotter

    o.setupCloseHandler()
    o.setupHeartbeat()

    log.Println("Creating containerd client")
    o.client, err = containerd.New(containerdAddress)
    if err != nil {
        log.Fatalf("Failed to start containerd client", err)
    }
    log.Println("Created containerd client")

    o.fcClient, err = fcclient.New(containerdTTRPCAddress)
    if err != nil {
        log.Fatalf("Failed to start firecracker client", err)
    }

    return o
}

func (o *Orchestrator) getImage(ctx context.Context, imageName string) (*containerd.Image, error) {
    image, found := o.cachedImages[imageName]
    if !found {
        var err error
        image, err = o.client.Pull(ctx, "docker.io/" + imageName,
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

func (o *Orchestrator) getVmConfig(vmID string, ni misc.NetworkInterface) (*proto.CreateVMRequest){
    kernelArgs := "ro noapic reboot=k panic=1 pci=off nomodules systemd.log_color=false systemd.unit=firecracker.target init=/sbin/overlay-init tsc=reliable quiet 8250.nr_uarts=0 ipv6.disable=1"

    return &proto.CreateVMRequest{
        VMID: vmID,
        KernelArgs: kernelArgs,
        MachineCfg: &proto.FirecrackerMachineConfiguration{
            VcpuCount:  1,
            MemSizeMib: 512,
        },
        NetworkInterfaces: []*proto.FirecrackerNetworkInterface{{
            StaticConfig: &proto.StaticNetworkConfiguration{
                MacAddress:  ni.MacAddress,
                HostDevName: ni.HostDevName,
                IPConfig: &proto.IPConfiguration{
                    PrimaryAddr: ni.PrimaryAddress + ni.Subnet,
                    GatewayAddr: ni.GatewayAddress,
                },
            },
        }},
    }
}

func (o *Orchestrator) ActiveVmExists(vmID string) (bool) {
    return o.vmPool.IsVmActive(vmID)
}

// FIXME: concurrent StartVM and StopVM with the same vmID would not work corrently.
// TODO: add state machine to VM struct

// accounted for races: start->start; need stop->start, start->stop, stop->stop
func (o *Orchestrator) StartVM(ctx context.Context, vmID, imageName string) (string, string, error) {
    var t_profile string
    var t_start, t_elapsed time.Time
    //log.Printf("Received: %v %v", vmID, imageName)

    vm, err := o.vmPool.Allocate(vmID)
    if err != nil {
        if _, ok := err.(*misc.AlreadyStartingErr); ok {
            return "VM " + vmID + " is already starting", t_profile, err
        }
    }

    vm.SetStateStarting()

    if vm.Ni, err = o.niPool.Allocate();  err != nil {
        return "No free NI available", t_profile, err
    }

    ctx = namespaces.WithNamespace(ctx, namespaceName)
    t_start = time.Now()
    if vm.Image, err = o.getImage(ctx, imageName); err != nil {
        return "Failed to start VM", t_profile, errors.Wrapf(err, "Failed to get/pull image")
    }
    t_elapsed = time.Now()
    t_profile += strconv.FormatInt(t_elapsed.Sub(t_start).Microseconds(), 10) + ";"

    t_start = time.Now()
    _, err = o.fcClient.CreateVM(ctx, o.getVmConfig(vmID, *vm.Ni))
    t_elapsed = time.Now()
    t_profile += strconv.FormatInt(t_elapsed.Sub(t_start).Microseconds(), 10) + ";"
    if err != nil {
        if errCleanup := o.cleanup(ctx, vmID, false, false, false); errCleanup != nil {
            log.Printf("Cleanup failed: ", errCleanup)
        }
        return "Failed to start VM", t_profile, errors.Wrap(err, "failed to create the VM")
    }

    t_start = time.Now()
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
    t_elapsed = time.Now()
    t_profile += strconv.FormatInt(t_elapsed.Sub(t_start).Microseconds(), 10) + ";"
    if err != nil {
        if errCleanup := o.cleanup(ctx, vmID, true, false, false); errCleanup != nil {
            log.Printf("Cleanup failed: ", errCleanup)
        }
        return "Failed to start VM", t_profile, errors.Wrap(err, "failed to create a container")
    }

    t_start = time.Now()
    task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
    vm.Task = &task
    t_elapsed = time.Now()
    t_profile += strconv.FormatInt(t_elapsed.Sub(t_start).Microseconds(), 10) + ";"
    if err != nil {
        if errCleanup := o.cleanup(ctx, vmID, true, true, false); errCleanup != nil {
            log.Printf("Cleanup failed: ", errCleanup)
        }
        return "Failed to start VM", t_profile, errors.Wrap(err, "failed to create a task")
    }

    t_start = time.Now()
    ch, err := task.Wait(ctx)
    vm.TaskCh = ch
    t_elapsed = time.Now()
    t_profile += strconv.FormatInt(t_elapsed.Sub(t_start).Microseconds(), 10) + ";"
    if err != nil {
        if errCleanup := o.cleanup(ctx, vmID, true, true, true); errCleanup != nil {
            log.Printf("Cleanup failed: ", errCleanup)
        }
        return "Failed to start VM", t_profile, errors.Wrap(err, "failed to wait for a task")
    }

    t_start = time.Now()
    if err := task.Start(ctx); err != nil {
        if errCleanup := o.cleanup(ctx, vmID, true, true, true); errCleanup != nil {
            log.Printf("Cleanup failed: ", errCleanup)
        }
        return "Failed to start VM", t_profile, errors.Wrap(err, "failed to start a task")
    }
    t_elapsed = time.Now()
    t_profile += strconv.FormatInt(t_elapsed.Sub(t_start).Microseconds(), 10) + ";"

    //log.Println("Connecting to the function in VM "+vmID+" ip:"+ni.PrimaryAddress)
    conn, err := grpc.Dial(vm.Ni.PrimaryAddress+":50051", grpc.WithInsecure(), grpc.WithBlock())
    vm.Conn = conn
    if err != nil {
        if errCleanup := o.cleanup(ctx, vmID, true, true, true); errCleanup != nil {
            log.Printf("Cleanup failed: ", errCleanup)
        }
        return "Failed to start VM", t_profile, errors.Wrap(err, "Failed to connect to a function")
    }
    funcClient := hpb.NewGreeterClient(conn)
    vm.FuncClient = &funcClient
    //log.Println("Connected to the function in VM "+vmID)

    vm.SetStateActive()
    log.Printf(vm)

    return "VM, container, and task started successfully", t_profile, nil
}

func (o *Orchestrator) GetFuncClient(vmID string) (*hpb.GreeterClient, error) {
    return o.vmPool.GetFuncClient(vmID)
}

func (o *Orchestrator) cleanup(ctx context.Context, vmID string, isVm, isCont, isTask bool) (error) {
    vm, err := o.vmPool.Free(vmID)
    if err != nil {
        if _, ok := err.(*misc.AlreadyDeactivatingErr); ok {
            return nil // not an error
        } else if _, ok := err.(*misc.DeactivatingErr); ok {
            return err
        } else {
            panic("Deallocation failed for an unknown reason")
        }
    }

    if isTask == true {
        task := *vm.Task
        if _, err := task.Delete(ctx); err != nil {
            return errors.Wrapf(err, "Attempt to delete the task failed.")
        }
    }
    if isCont == true {
        cont := *vm.Container
        if err := cont.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
            return errors.Wrapf(err, "Attempt to delete the container failed.")
        }
    }

    if isVm == true {
        if _, err := o.fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID}); err != nil {
            return errors.Wrapf(err, "Attempt to stop the VM failed.")
        }
    }

    o.niPool.Free(vm.Ni)

    return nil
}

// Note: VMs are not quisced before being stopped
func (o *Orchestrator) StopSingleVM(ctx context.Context, vmID string) (string, error) {
    ctx = namespaces.WithNamespace(ctx, namespaceName)

    vm, err := o.vmPool.Free(vmID)
    if err != nil {
        if _, ok := err.(*misc.AlreadyDeactivatingErr); ok {
            return "VM " + vmID + " is already being deactivated", nil // not an error
        } else if _, ok := err.(*misc.DeactivatingErr); ok {
            return "Error while deallocating VM", err
        } else {
            panic("Deallocation failed for an unknown reason")
        }
    }

    if err := vm.Conn.Close(); err != nil {
        log.Println("Failed to close the connection to function: ", err)
        return "Closing connection to function in VM " + vmID + " failed", err
    }
    task := *vm.Task
    if err := task.Kill(ctx, syscall.SIGKILL); err != nil {
        log.Println("Failed to kill the task: ", err)
        return "Killing task of VM " + vmID + " failed", err
    }
    status := <-vm.TaskCh
    if _, _, err := status.Result(); err != nil {
	return "Waiting for task termination failed of the VM " + vmID, err
    }
    if _, err := task.Delete(ctx); err != nil {
        log.Println("failed to delete the task of the VM: ", err)
        return "Deleting task of VM " + vmID + " failed", err
    }
    container := *vm.Container
    if err := container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
        log.Println("failed to delete the container of the VM: ", err)
        return "Deleting container of VM " + vmID + " failed", err
    }
    log.Println("Stopping the VM" + vmID)
    if _, err := o.fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID}); err != nil {
        log.Println("failed to stop the VM: ", err)
        return "Stopping VM " + vmID + " failed", err
    }

    o.niPool.Free(vm.Ni)

    return "VM " + vmID + " stopped successfully", nil
}

func (o *Orchestrator) StopActiveVMs() (error) {
    var vmGroup sync.WaitGroup
    for vmID, vm := range o.vmPool.GetVmMap() {
        vmGroup.Add(1)
        go func(vmID string, vm misc.VM) {
            defer vmGroup.Done()
            message, err := o.StopSingleVM(context.Background(), vmID)
            if err != nil {
                log.Printf(message, err)
            }
            log.Println(message)
        }(vmID, vm)
    }

    log.Println("waiting for goroutines")
    vmGroup.Wait()
    log.Println("waiting done")

    log.Println("Closing fcClient")
    o.fcClient.Close()
    log.Println("Closing containerd client")
    o.client.Close()

    return nil
}

func (o *Orchestrator) setupHeartbeat() {
    var heartbeat *time.Ticker
    heartbeat = time.NewTicker(60 * time.Second)
    go func() {
        for {
            select {
            case <-heartbeat.C:
                log.Printf("HEARTBEAT: %v VMs are active\n", len(o.vmPool.GetVmMap()))
            } // select
        } // for
    }() // go func
}

func (o *Orchestrator) setupCloseHandler() {
    c := make(chan os.Signal, 2)
    signal.Notify(c, os.Interrupt, syscall.SIGTERM)
    go func() {
        <-c
        log.Println("\r- Ctrl+C pressed in Terminal")
        o.StopActiveVMs()
        os.Exit(0)
    }()
}
