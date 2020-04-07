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

//    store, err = skv.Open("/var/lib/fccd-orchestrator/vms.db")
//    if err != nil { log.Fatalf("Failed to open db file", err) }

    return o
}

func (o *Orchestrator) niAlloc() (*misc.NetworkInterface, error) {
    var ni misc.NetworkInterface
    o.mu.Lock()
    defer o.mu.Unlock()
    if len(o.niList) == 0 {
        return nil, errors.New("No NI available")
    }
    ni, o.niList = o.niList[len(o.niList)-1], o.niList[:len(o.niList)-1] // pop

    return &ni, nil
}

func (o *Orchestrator) niFree(ni misc.NetworkInterface) {
    o.mu.Lock()
    defer o.mu.Unlock()
    o.niList = append(o.niList, ni)
}

func (o *Orchestrator) cleanup(ctx context.Context, vmID string, isVm, isCont, isTask bool) (error) {
    vm, _ := o.active_vms[vmID]
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

    o.niFree(*vm.Ni)

    return nil
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

func (o *Orchestrator) activateVM(vmID string, vm misc.VM) {
    o.active_vms[vmID] = vm // TBD
}
/*
func (o *Orchestrator) activateVM(
    vmID string,
    image *containerd.Image,
    container *containerd.Container,
    task *containerd.Task,
    exitStatusCh <-chan containerd.ExitStatus,
    ni *misc.NetworkInterface,
    conn *grpc.ClientConn,
    funcClient *hpb.GreeterClient) {
    o.mu.Lock()
    o.active_vms[vmID] = misc.VM{
        Image: image,
        Container: container,
        Task: task,
        Ni: ni,
        Conn: conn,
        FuncClient: funcClient,
    }
    o.mu.Unlock()
}
*/

func (o *Orchestrator) deactivateVM(vmID string) {
    o.mu.Lock()
    vm, _ := o.active_vms[vmID]
    o.niList = append(o.niList, *vm.Ni) // TODO: make FIFO
    delete(o.active_vms, vmID)
    o.mu.Unlock()
}
// FIXME: concurrent StartVM and StopVM with the same vmID would not work corrently.
// TODO: add state machine to VM struct
func (o *Orchestrator) StartVM(ctx context.Context, vmID, imageName string) (string, string, error) {
    var t_profile string
    var err error
    var t_start, t_elapsed time.Time
    var vm misc.VM
    //log.Printf("Received: %v %v", vmID, imageName)

    if _, is_present := o.active_vms[vmID]; is_present {
        log.Printf("VM %v is among active VMs", vmID)
        return "VM " + vmID + " already active", t_profile, errors.New("VM exists")
    }

    ctx = namespaces.WithNamespace(ctx, namespaceName)
    t_start = time.Now()
    if vm.Image, err = o.getImage(ctx, imageName); err != nil {
        return "Failed to start VM", t_profile, errors.Wrapf(err, "Failed to get/pull image")
    }
    t_elapsed = time.Now()
    t_profile += strconv.FormatInt(t_elapsed.Sub(t_start).Microseconds(), 10) + ";"

    if vm.Ni, err = o.niAlloc();  err != nil {
        return "No free NI available", t_profile, err
    }

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

    o.activateVM(vmID, vm)

    return "VM, container, and task started successfully", t_profile, nil
}

type NonExistErr string

func (e NonExistErr) Error() string {
    return fmt.Sprintf("%s does not exist", e)
}

func (o *Orchestrator) GetFuncClientByID(vmID string) (*hpb.GreeterClient, error) {
    vm, is_present := o.active_vms[vmID]
    if !is_present { return nil, NonExistErr("FuncClient") }

    return vm.FuncClient, nil
}

func (o *Orchestrator) IsVMActive(vmID string) (bool) {
    _, is_active := o.active_vms[vmID]
    return is_active
}

func (o *Orchestrator) StopSingleVM(ctx context.Context, vmID string) (string, error) {
    ctx = namespaces.WithNamespace(ctx, namespaceName)

    vm, is_present := o.active_vms[vmID]
    if !is_present {
        log.Printf("VM %v is not recorded as an active VM, attempting a force stop.", vmID)
        log.Println("Stopping the VM" + vmID)
        if _, err := o.fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID}); err != nil {
            log.Printf("failed to stop the VM, err: %v\n", err)
            return "Stopping VM " + vmID + " failed", err
        }

        o.niFree(*vm.Ni)

        return "VM " + vmID + " stopped forcefully but successfully", nil
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

    o.deactivateVM(vmID)

    return "VM " + vmID + " stopped successfully", nil
}

func (o *Orchestrator) StopActiveVMs() error {
    var vmGroup sync.WaitGroup
    for vmID, vm := range o.active_vms {
        vmGroup.Add(1)
        go func(vmID string, vm misc.VM) {
            defer vmGroup.Done()
            ctx := namespaces.WithNamespace(context.Background(), namespaceName)
            ctx, _ = context.WithDeadline(ctx, time.Now().Add(time.Duration(300) * time.Second))
	    if err := vm.Conn.Close(); err != nil {
		log.Println("Failed to close the connection to function: ", err)
	    }
            task := *vm.Task
            if err := task.Kill(ctx, syscall.SIGKILL); err != nil {
                log.Printf("Failed to kill the task, err: %v\n", err)
            }
            status := <-vm.TaskCh
            if _, _, err := status.Result(); err != nil {
                log.Printf("Waiting for task termination failed of the VM " + vmID, err)
            }
            if _, err := task.Delete(ctx); err != nil {
                log.Printf("failed to delete the task of the VM, err: %v\n", err)
            }
            container := *vm.Container
            if err := container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
                log.Printf("failed to delete the container of the VM, err: %v\n", err)
            }

            if _, err := o.fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID}); err != nil {
                log.Printf("failed to stop the VM, err: %v\n", err)
            }
            o.deactivateVM(vmID)
            log.Println("Stopping the VM" + vmID)
        }(vmID, vm)
    }

    log.Println("waiting for goroutines")
    vmGroup.Wait()
    log.Println("waiting done")

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
            PrimaryAddress: fmt.Sprintf("19%d.128.%d.%d", i%2+6, (i+2)/256, (i+2)%256),
            Subnet: "/10",
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
