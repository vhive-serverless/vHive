// Copyright 2018-2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package main

import (
    "context"
    "flag"
    _ "fmt"
    _ "io/ioutil"
    "io"
    "log"
    "net"
    "syscall"
    _ "time"
    "math/rand"
    "strconv"

    "github.com/containerd/containerd"
    "github.com/containerd/containerd/cio"
    "github.com/containerd/containerd/namespaces"
    "github.com/containerd/containerd/oci"
    "github.com/pkg/errors"

    fcclient "github.com/firecracker-microvm/firecracker-containerd/firecracker-control/client"
    "github.com/firecracker-microvm/firecracker-containerd/proto"
    "github.com/firecracker-microvm/firecracker-containerd/runtime/firecrackeroci"

    "google.golang.org/grpc"
    pb "github.com/ustiugov/fccd-orchestrator/proto"
    "os"
    "os/signal"
    "sync"
    "time"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"

//    "github.com/ustiugov/skv"
)

const (
    containerdAddress      = "/run/firecracker-containerd/containerd.sock"
    containerdTTRPCAddress = containerdAddress + ".ttrpc"
    namespaceName          = "firecracker-containerd"

    port = ":3333"
)

type VM struct {
    Image containerd.Image
    Container containerd.Container
    Task containerd.Task
}

var flog *os.File
//var store *skv.KVStore //TODO: stop individual VMs
var active_vms map[string]VM
var snapshotter *string
var client *containerd.Client
var fcClient *fcclient.Client
var ctx context.Context
var mu = &sync.Mutex{}

type myWriter struct {
    io.Writer
}

func (m *myWriter) Write(p []byte) (n int, err error) {
    n, err = m.Writer.Write(p)

    if flusher, ok := m.Writer.(interface{ Flush() }); ok {
        flusher.Flush()
    } else if syncer := m.Writer.(interface{ Sync() error }); ok {
        // Preserve original error
        if err2 := syncer.Sync(); err2 != nil && err == nil {
            err = err2
        }
    }
    return
}

func main() {
    var err error

    rand.Seed(42)
    snapshotter = flag.String("ss", "devmapper", "snapshotter")

    if flog, err = os.Create("/tmp/fccd.log"); err != nil {
        panic(err)
    }
    defer flog.Close()

    //log.SetOutput(&myWriter{Writer: flog})
    log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
    flag.Parse()

    setupCloseHandler()
    setupHeartbeat()

//    store, err = skv.Open("/var/lib/fccd-orchestrator/vms.db")
//    if err != nil { log.Fatalf("Failed to open db file", err) }

    active_vms = make(map[string]VM)

    log.Println("Creating containerd client")
    client, err = containerd.New(containerdAddress)
    if err != nil {
        log.Fatalf("Failed to start containerd client", err)
    }
    log.Println("Created containerd client")

    ctx = namespaces.WithNamespace(context.Background(), namespaceName)

    fcClient, err = fcclient.New(containerdTTRPCAddress)
    if err != nil {
        log.Fatalf("Failed to start firecracker client", err)
    }

    lis, err := net.Listen("tcp", port)
    if err != nil {
        log.Fatalf("failed to listen: %v", err)
    }
    s := grpc.NewServer()
    pb.RegisterOrchestratorServer(s, &server{})

    log.Println("Listening on port" + port)
    if err := s.Serve(lis); err != nil {
        log.Fatalf("failed to serve: %v", err)
    }
}

type server struct {
    pb.UnimplementedOrchestratorServer
}

func (s *server) StartVM(ctx_ context.Context, in *pb.StartVMReq) (*pb.Status, error) {
    vmID := in.GetId()
    log.Printf("Received: %v %v", vmID, in.GetImage())
    if _, is_present := active_vms[vmID]; is_present {
        log.Printf("VM %v is among active VMs", vmID)
        return &pb.Status{Message: "VM " + vmID + " already active"}, nil //err
    }
/*    var VM_ VM
    if err := store.Get(vmID, &VM_); err == nil {
        return &pb.Status{Message: "VM " + vmID + " already exists in db"}, nil //err
    } else if err != skv.ErrNotFound {
        log.Printf("Get VM from db returned error: %v\n", err)
        return &pb.Status{Message: "Get VM " + vmID + " from db failed"}, err
    }
*/
    image, err := client.Pull(ctx, "docker.io/" + in.GetImage(),
                              containerd.WithPullUnpack,
                              containerd.WithPullSnapshotter(*snapshotter),
                             )
    if err != nil {
        return &pb.Status{Message: "Pulling a VM image failed"}, errors.Wrapf(err, "creating container")
    }

    netID, err := strconv.Atoi(vmID)
    if err != nil {
        log.Println("vmID must be be numeric", err)
        return &pb.Status{Message: "vmID must be numeric"}, err
    } else { netID = netID % 2 + 1 }

    createVMRequest := &proto.CreateVMRequest{
        VMID: vmID,
        MachineCfg: &proto.FirecrackerMachineConfiguration{
            VcpuCount:  1,
            MemSizeMib: 512,
        },
        NetworkInterfaces: []*proto.FirecrackerNetworkInterface{{
            CNIConfig: &proto.CNIConfiguration{
                NetworkName: "fcnet"+strconv.Itoa(netID),
                InterfaceName: "veth0",
            },
        }},
    }

    _, err = fcClient.CreateVM(ctx, createVMRequest)
    if err != nil {
        errStatus, _ := status.FromError(err)
        log.Printf("fcClient failed to create a VM", err)
        if errStatus.Code() != codes.AlreadyExists {
            _, err1 := fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID})
            if err1 != nil { log.Printf("Attempt to clean up failed after creating a VM failed.", err1) }
        }
        return &pb.Status{Message: "Failed to start VM"}, errors.Wrap(err, "failed to create the VM")
    }
    container, err := client.NewContainer(
                                          ctx,
                                          vmID,
                                          containerd.WithSnapshotter(*snapshotter),
                                          containerd.WithNewSnapshot(vmID, image),
                                          containerd.WithNewSpec(
                                                                 oci.WithImageConfig(image),
                                                                 firecrackeroci.WithVMID(vmID),
                                                                 firecrackeroci.WithVMNetwork,
                                                                ),
                                          containerd.WithRuntime("aws.firecracker", nil),
                                         )
    if err != nil {
        log.Printf("Failed to create a container", err)
        if _, err1 := fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID}); err1 != nil {
            log.Printf("Attempt to stop the VM failed after creating container had failed.", err1)
        }
        return &pb.Status{Message: "Failed to start container for the VM" + vmID }, err
    }
    task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
    if err != nil {
        log.Printf("Failed to create a task", err)
        if err1 := container.Delete(ctx, containerd.WithSnapshotCleanup); err1 != nil {
            log.Printf("Attempt to delete the container failed after creating the task had failed.")
        }
        if _, err1 := fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID}); err1 != nil {
            log.Printf("Attempt to stop the VM failed after creating the task had failed.", err1)
        }
        return &pb.Status{Message: "Failed to create the task for the VM" + vmID }, err

    }

    log.Printf("Successfully created task: %s for the container\n", task.ID())
    _, err = task.Wait(ctx)
    if err != nil {
        if _, err1 := task.Delete(ctx); err1 != nil {
            log.Printf("Attempt to delete the task failed after waiting for the task had failed.")
        }
        if err1 := container.Delete(ctx, containerd.WithSnapshotCleanup); err1 != nil {
            log.Printf("Attempt to delete the container failed after waiting for the task had failed.")
        }
        if _, err1 := fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID}); err1 != nil {
            log.Printf("Attempt to stop the VM failed after waiting for the task had failed.", err1)
        }
        return &pb.Status{Message: "Failed to wait for the task for the VM" + vmID }, err

    }

    if err := task.Start(ctx); err != nil {
        if _, err1 := task.Delete(ctx); err1 != nil {
            log.Printf("Attempt to delete the task failed after starting the task had failed.")
        }
        if err1 := container.Delete(ctx, containerd.WithSnapshotCleanup); err1 != nil {
            log.Printf("Attempt to delete the container failed after starting the task had failed.")
        }
        if _, err1 := fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID}); err1 != nil {
            log.Printf("Attempt to stop the VM failed after starting the task had failed.", err1)
        }
        return &pb.Status{Message: "Failed to start the task for the VM" + vmID }, err

    }

    log.Println("Successfully started the container task for the VM", vmID)

    mu.Lock()
    active_vms[vmID] = VM{Image: image, Container: container, Task: task}
    mu.Unlock()
/*
    if err := store.Put(vmID, vmID); err != nil {
        log.Printf("Failed to save VM attributes, err:%v\n", err)
    }
*/
    //TODO: set up port forwarding to a private IP, get IP

    return &pb.Status{Message: "started VM " + vmID }, nil
}

func (s *server) StopSingleVM(ctx_ context.Context, in *pb.StopSingleVMReq) (*pb.Status, error) {
    vmID := in.GetId()
    log.Printf("Received stop single VM request for VM %v", vmID)
    if vm, is_present := active_vms[vmID]; is_present {
        if err := vm.Task.Kill(ctx, syscall.SIGKILL); err != nil {
            log.Printf("Failed to kill the task, err: %v\n", err)
            return &pb.Status{Message: "Killinh task of VM " + vmID + " failed"}, err
        }
        if _, err := vm.Task.Delete(ctx); err != nil {
            log.Printf("failed to delete the task of the VM, err: %v\n", err)
            return &pb.Status{Message: "Deleting task of VM " + vmID + " failed"}, err
        }
        if err := vm.Container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
            log.Printf("failed to delete the container of the VM, err: %v\n", err)
            return &pb.Status{Message: "Deleting container of VM " + vmID + " failed"}, err
        }

        mu.Lock() // CreateVM may fail when invoked by multiple threads/goroutines
        log.Println("Stopping the VM" + vmID)
        if _, err := fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID}); err != nil {
            log.Printf("failed to stop the VM, err: %v\n", err)
            return &pb.Status{Message: "Stopping VM " + vmID + " failed"}, err
        }
        delete(active_vms, vmID)
        mu.Unlock()
/*        if err := store.Delete(vmID); err != skv.ErrNotFound {
            return &pb.Status{Message: "Removed VM " + vmID + " from the db"}, nil //err
        } else if err != nil {
            log.Printf("Get VM from db returned error: %v\n", err)
            return &pb.Status{Message: "Get VM " + vmID + " from db failed"}, err
        }
*/
    } else {
        log.Printf("VM %v is not recorded as an active VM, attempting a force stop.", vmID)
        mu.Lock() // CreateVM may fail when invoked by multiple threads/goroutines
        log.Println("Stopping the VM" + vmID)
        if _, err := fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID}); err != nil {
            log.Printf("failed to stop the VM, err: %v\n", err)
            return &pb.Status{Message: "Stopping VM " + vmID + " failed"}, err
        }
        mu.Unlock()
    }
    return &pb.Status{Message: "Stopped VM"}, nil
}

func (s *server) StopVMs(ctx_ context.Context, in *pb.StopVMsReq) (*pb.Status, error) {
    log.Printf("Received StopVMs request")
    err := stopActiveVMs()
    if err != nil {
        log.Printf("Failed to stop VMs, err: %v\n", err)
        return &pb.Status{Message: "Failed to stop VMs"}, err
    }
    os.Exit(0)
    return &pb.Status{Message: "Stopped VMs"}, nil
}

func stopActiveVMs() error { // (*pb.Status, error) {
    var vmGroup sync.WaitGroup
    for vmID, vm := range active_vms {
        vmGroup.Add(1)
        go func(vmID string, vm VM) {
            defer vmGroup.Done()
            if err := vm.Task.Kill(ctx, syscall.SIGKILL); err != nil {
                log.Printf("Failed to kill the task, err: %v\n", err)
                //return &pb.Status{Message: "Killinh task of VM " + vmID + " failed"}, err
            }
            if _, err := vm.Task.Delete(ctx); err != nil {
                log.Printf("failed to delete the task of the VM, err: %v\n", err)
                //return &pb.Status{Message: "Deleting task of VM " + vmID + " failed"}, err
            }
            if err := vm.Container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
                log.Printf("failed to delete the container of the VM, err: %v\n", err)
                //return &pb.Status{Message: "Deleting container of VM " + vmID + " failed"}, err
            }

            mu.Lock() // CreateVM may fail when invoked by multiple threads/goroutines
            log.Println("Stopping the VM" + vmID)
            if _, err := fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID}); err != nil {
                log.Printf("failed to stop the VM, err: %v\n", err)
                //return &pb.Status{Message: "Stopping VM " + vmID + " failed"}, err
            }
            mu.Unlock()
/*            if err := store.Delete(vmID); err != skv.ErrNotFound {
                delete(active_vms, vmID)
                //return &pb.Status{Message: "Removed VM " + vmID + " from the db"}, nil //err
            } else if err != nil {
                log.Printf("Get VM from db returned error: %v\n", err)
                //return &pb.Status{Message: "Get VM " + vmID + " from db failed"}, err
            }
*/
        }(vmID, vm)
    }
    log.Println("waiting for goroutines")
    vmGroup.Wait()
    log.Println("waiting done")

    log.Println("Closing fcClient")
    fcClient.Close()
    log.Println("Closing containerd client")
    client.Close()
//    store.Close()
    return nil //&pb.Status{Message: "Stopped active VMs"}, nil
}

func setupHeartbeat() {
    var heartbeat *time.Ticker
    heartbeat = time.NewTicker(60 * time.Second)
    go func() {
        for {
            select {
            case <-heartbeat.C:
                log.Printf("HEARTBEAT: %v VMs are active\n", len(active_vms))
            } // select
        } // for
    }() // go func
}

func setupCloseHandler() {
    c := make(chan os.Signal, 2)
    signal.Notify(c, os.Interrupt, syscall.SIGTERM)
    go func() {
        <-c
        log.Println("\r- Ctrl+C pressed in Terminal")
        stopActiveVMs()
        os.Exit(0)
    }()
}
