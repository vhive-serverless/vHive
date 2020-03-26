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
    "fmt"
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

    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"

    "github.com/ustiugov/fccd-orchestrator/misc"

//    "github.com/ustiugov/skv"
)

const (
    containerdAddress      = "/run/firecracker-containerd/containerd.sock"
    containerdTTRPCAddress = containerdAddress + ".ttrpc"
    namespaceName          = "firecracker-containerd"
)

type Orchestrator struct {
    active_vms map[string]misc.VM
    cachedImages map[string]containerd.Image
    niList []misc.NetworkInterface
    snapshotter string
    client *containerd.Client
    fcClient *fcclient.Client
    mu *sync.Mutex
// store *skv.KVStore
}

func NewOrchestrator(snapshotter string, niNum int) *Orchestrator {
    var err error

    o := new(Orchestrator)
    o.active_vms = make(map[string]misc.VM)
    o.cachedImages= make(map[string]containerd.Image)
    o.generateNetworkInterfaceNames(niNum)
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

    o.mu = &sync.Mutex{}

    return o
}

func (o *Orchestrator) StartVM(ctx context.Context, vmID, imageName string) (string, string, error) {
    var t_profile string
    var err error
    var image containerd.Image
    var t_start, t_elapsed time.Time
    log.Printf("Received: %v %v", vmID, imageName)

    if _, is_present := o.active_vms[vmID]; is_present {
        log.Printf("VM %v is among active VMs", vmID)
        return "VM " + vmID + " already active", t_profile, nil
    }

/*    var VM_ VM
    if err := store.Get(vmID, &VM_); err == nil {
        return &pb.StartVMResp{Message: "VM " + vmID + " already exists in db"}, nil //err
    } else if err != skv.ErrNotFound {
        log.Printf("Get VM from db returned error: %v\n", err)
        return &pb.StartVMResp{Message: "Get VM " + vmID + " from db failed"}, err
    }
*/
    ctx = namespaces.WithNamespace(ctx, namespaceName)
    ctx, _ = context.WithDeadline(ctx, time.Now().Add(time.Duration(60) * time.Second))
    t_start = time.Now()
    image, found := o.cachedImages[imageName]
    if !found {
        image, err = o.client.Pull(ctx, "docker.io/" + imageName,
            containerd.WithPullUnpack,
            containerd.WithPullSnapshotter(o.snapshotter),
        )
        if err != nil {
            return "Pulling a VM image failed", t_profile, errors.Wrapf(err, "creating container")
        }
        o.cachedImages[imageName] = image
    }
    t_elapsed = time.Now()
    t_profile += strconv.FormatInt(t_elapsed.Sub(t_start).Microseconds(), 10) + ";"
/*
    netID, err := strconv.Atoi(vmID)
    if err != nil {
        log.Println("vmID must be be numeric", err)
        return &pb.StartVMResp{Message: "vmID must be numeric"}, err
    } else { netID = netID % 2 + 1 }
*/

    o.mu.Lock()
    var ni misc.NetworkInterface
    ni, o.niList = o.niList[len(o.niList)-1], o.niList[:len(o.niList)-1] // pop
    o.mu.Unlock()

    kernelArgs := "ro noapic reboot=k panic=1 pci=off nomodules systemd.log_color=false systemd.unit=firecracker.target init=/sbin/overlay-init tsc=reliable quiet 8250.nr_uarts=0 ipv6.disable=1"
    createVMRequest := &proto.CreateVMRequest{
        VMID: vmID,
        KernelArgs: kernelArgs,
        MachineCfg: &proto.FirecrackerMachineConfiguration{
            VcpuCount:  1,
            MemSizeMib: 512,
        },
/* FIXME: CNI config assigns IPs dynamizally and fcclient does not return them
        NetworkInterfaces: []*proto.FirecrackerNetworkInterface{{
            CNIConfig: &proto.CNIConfiguration{
                NetworkName: "fcnet"+strconv.Itoa(netID),
                InterfaceName: "veth0",
            },
        }},
*/
        NetworkInterfaces: []*proto.FirecrackerNetworkInterface{{
            StaticConfig: &proto.StaticNetworkConfiguration{
                MacAddress:  ni.MacAddress,
                HostDevName: ni.HostDevName,
                IPConfig: &proto.IPConfiguration{
                    PrimaryAddr: ni.PrimaryAddress,
                    GatewayAddr: ni.GatewayAddress,
                },
            },
        }},
    }

    ctx, _ = context.WithDeadline(ctx, time.Now().Add(time.Duration(120) * time.Second))
    t_start = time.Now()
    _, err = o.fcClient.CreateVM(ctx, createVMRequest)
    t_elapsed = time.Now()
    t_profile += strconv.FormatInt(t_elapsed.Sub(t_start).Microseconds(), 10) + ";"
    if err != nil {
        errStatus, _ := status.FromError(err)
        log.Printf("fcClient failed to create a VM", err)
        if errStatus.Code() != codes.AlreadyExists {
            _, err1 := o.fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID})
            if err1 != nil { log.Printf("Attempt to clean up failed after creating a VM failed.", err1) }
        }
        return "Failed to start VM", t_profile, errors.Wrap(err, "failed to create the VM")
    }
    ctx, _ = context.WithDeadline(ctx, time.Now().Add(time.Duration(5) * time.Second))
    t_start = time.Now()
    container, err := o.client.NewContainer(
                                          ctx,
                                          vmID,
                                          containerd.WithSnapshotter(o.snapshotter),
                                          containerd.WithNewSnapshot(vmID, image),
                                          containerd.WithNewSpec(
                                                                 oci.WithImageConfig(image),
                                                                 firecrackeroci.WithVMID(vmID),
                                                                 firecrackeroci.WithVMNetwork,
                                                                ),
                                          containerd.WithRuntime("aws.firecracker", nil),
                                         )
    t_elapsed = time.Now()
    t_profile += strconv.FormatInt(t_elapsed.Sub(t_start).Microseconds(), 10) + ";"
    if err != nil {
        log.Printf("Failed to create a container", err)
        if _, err1 := o.fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID}); err1 != nil {
            log.Printf("Attempt to stop the VM failed after creating container had failed.", err1)
        }
        return "Failed to start container for the VM" + vmID, t_profile, err
    }
    ctx, _ = context.WithDeadline(ctx, time.Now().Add(time.Duration(5) * time.Second))
    t_start = time.Now()
    task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
    t_elapsed = time.Now()
    t_profile += strconv.FormatInt(t_elapsed.Sub(t_start).Microseconds(), 10) + ";"
    if err != nil {
        log.Printf("Failed to create a task", err)
        if err1 := container.Delete(ctx, containerd.WithSnapshotCleanup); err1 != nil {
            log.Printf("Attempt to delete the container failed after creating the task had failed.")
        }
        if _, err1 := o.fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID}); err1 != nil {
            log.Printf("Attempt to stop the VM failed after creating the task had failed.", err1)
        }
        return "Failed to create the task for the VM" + vmID, t_profile, err

    }

    //log.Printf("Successfully created task: %s for the container\n", task.ID())
    t_start = time.Now()
    _, err = task.Wait(ctx)
    t_elapsed = time.Now()
    t_profile += strconv.FormatInt(t_elapsed.Sub(t_start).Microseconds(), 10) + ";"
    if err != nil {
        if _, err1 := task.Delete(ctx); err1 != nil {
            log.Printf("Attempt to delete the task failed after waiting for the task had failed.")
        }
        if err1 := container.Delete(ctx, containerd.WithSnapshotCleanup); err1 != nil {
            log.Printf("Attempt to delete the container failed after waiting for the task had failed.")
        }
        if _, err1 := o.fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID}); err1 != nil {
            log.Printf("Attempt to stop the VM failed after waiting for the task had failed.", err1)
        }
        return "Failed to wait for the task for the VM" + vmID, t_profile, err

    }

    ctx, _ = context.WithDeadline(ctx, time.Now().Add(time.Duration(5) * time.Second))
    t_start = time.Now()
    if err := task.Start(ctx); err != nil {
        if _, err1 := task.Delete(ctx); err1 != nil {
            log.Printf("Attempt to delete the task failed after starting the task had failed.")
        }
        if err1 := container.Delete(ctx, containerd.WithSnapshotCleanup); err1 != nil {
            log.Printf("Attempt to delete the container failed after starting the task had failed.")
        }
        if _, err1 := o.fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID}); err1 != nil {
            log.Printf("Attempt to stop the VM failed after starting the task had failed.", err1)
        }
        return "Failed to start the task for the VM" + vmID, t_profile, err

    }
    t_elapsed = time.Now()
    t_profile += strconv.FormatInt(t_elapsed.Sub(t_start).Microseconds(), 10) + ";"

    //log.Println("Successfully started the container task for the VM", vmID)

    o.mu.Lock()
    o.active_vms[vmID] = misc.VM{Image: image, Container: container, Task: task, Ni: ni}
    o.mu.Unlock()
/*
    if err := store.Put(vmID, vmID); err != nil {
        log.Printf("Failed to save VM attributes, err:%v\n", err)
    }
*/
    return "VM, container, and task started successfully", t_profile, nil
}

func (o *Orchestrator) StopSingleVM(ctx context.Context, vmID string) (string, error) {
    ctx = namespaces.WithNamespace(ctx, namespaceName)

    vm, is_present := o.active_vms[vmID]
    if !is_present {
        log.Printf("VM %v is not recorded as an active VM, attempting a force stop.", vmID)
        o.mu.Lock() // CreateVM may fail when invoked by multiple threads/goroutines
        log.Println("Stopping the VM" + vmID)
        if _, err := o.fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID}); err != nil {
            log.Printf("failed to stop the VM, err: %v\n", err)
            return "Stopping VM " + vmID + " failed", err
        }

        o.niList = append(o.niList, vm.Ni)
        o.mu.Unlock()
        return "VM " + vmID + " stopped forcefully but successfully", nil
    }

    ctx, _ = context.WithDeadline(ctx, time.Now().Add(time.Duration(60) * time.Second))
    if err := vm.Task.Kill(ctx, syscall.SIGKILL); err != nil {
        log.Printf("Failed to kill the task, err: %v\n", err)
        return "Killing task of VM " + vmID + " failed", err
    }
    if _, err := vm.Task.Wait(ctx); err != nil {
        log.Printf("Failed to wait for the task to be killed, err: %v\n", err)
        return "Killing (waiting) task of VM " + vmID + " failed", err
    }
    if _, err := vm.Task.Delete(ctx); err != nil {
        log.Printf("failed to delete the task of the VM, err: %v\n", err)
        return "Deleting task of VM " + vmID + " failed", err
    }
    if err := vm.Container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
        log.Printf("failed to delete the container of the VM, err: %v\n", err)
        return "Deleting container of VM " + vmID + " failed", err
    }

    o.mu.Lock() // CreateVM may fail when invoked by multiple threads/goroutines
    log.Println("Stopping the VM" + vmID)
    if _, err := o.fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID}); err != nil {
        log.Printf("failed to stop the VM, err: %v\n", err)
        return "Stopping VM " + vmID + " failed", err
    }
    o.niList = append(o.niList, vm.Ni)
    delete(o.active_vms, vmID)
    o.mu.Unlock()
    /*        if err := store.Delete(vmID); err != skv.ErrNotFound {
        return &pb.Status{Message: "Removed VM " + vmID + " from the db"}, nil //err
    } else if err != nil {
        log.Printf("Get VM from db returned error: %v\n", err)
        return &pb.Status{Message: "Get VM " + vmID + " from db failed"}, err
    }
    */

    return "VM " + vmID + " stopped successfully", nil
}

func (o *Orchestrator) StopActiveVMs() error {
    ch := make(chan string, len(o.active_vms))

    for vmID, vm := range o.active_vms {
        go func(vmID string, vm misc.VM, ch chan string) {
            ctx := namespaces.WithNamespace(context.Background(), namespaceName)
            ctx, _ = context.WithDeadline(ctx, time.Now().Add(time.Duration(300) * time.Second))
            if err := vm.Task.Kill(ctx, syscall.SIGKILL); err != nil {
                log.Printf("Failed to kill the task, err: %v\n", err)
            }
            if _, err := vm.Task.Delete(ctx); err != nil {
                log.Printf("failed to delete the task of the VM, err: %v\n", err)
            }
            if err := vm.Container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
                log.Printf("failed to delete the container of the VM, err: %v\n", err)
            }

            o.mu.Lock() // CreateVM may fail when invoked by multiple threads/goroutines
            if _, err := o.fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID}); err != nil {
                log.Printf("failed to stop the VM, err: %v\n", err)
            }
            o.niList = append(o.niList, vm.Ni)
            delete(o.active_vms, vmID)
            o.mu.Unlock()
/*            if err := store.Delete(vmID); err != skv.ErrNotFound {
                delete(active_vms, vmID)
            } else if err != nil {
                log.Printf("Get VM from db returned error: %v\n", err)
            }
*/
            ch <- "Stopped VM " + vmID
        }(vmID, vm, ch)
    }

    for s := range ch {
        log.Println(s)
    }

    log.Println("Closing fcClient")
    o.fcClient.Close()
    log.Println("Closing containerd client")
    o.client.Close()
//    store.Close()
    return nil
}

func (o *Orchestrator) generateNetworkInterfaceNames(num int) {
    for i := 0; i < num; i++ {
        ni := misc.NetworkInterface{
            MacAddress: fmt.Sprintf("02:FC:00:00:%02X:%02X", i/256, i%256),
            HostDevName: fmt.Sprintf("fc-%d-tap0", i),
            PrimaryAddress: fmt.Sprintf("19%d.128.%d.%d/10", i%2+6, (i+2)/256, (i+2)%256),
            GatewayAddress: fmt.Sprintf("19%d.128.0.1", i%2+6),
        }
        //fmt.Println(ni)
        o.niList = append(o.niList, ni)
    }
    //os.Exit(0)
    return
}

func (o *Orchestrator) setupHeartbeat() {
    var heartbeat *time.Ticker
    heartbeat = time.NewTicker(60 * time.Second)
    go func() {
        for {
            select {
            case <-heartbeat.C:
                log.Printf("HEARTBEAT: %v VMs are active\n", len(o.active_vms))
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
