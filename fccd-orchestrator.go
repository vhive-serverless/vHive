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
)

const (
	containerdAddress      = "/run/firecracker-containerd/containerd.sock"
	containerdTTRPCAddress = containerdAddress + ".ttrpc"
	namespaceName          = "firecracker-containerd"

        port = ":3333"
)

type VM struct {
    Ctx context.Context
    Image containerd.Image
    Container containerd.Container
    Task containerd.Task
    VMID string
}

var active_vms []VM
var snapshotter *string
var client *containerd.Client
var fcClient *fcclient.Client
var ctx context.Context
var g_err error
var mu = &sync.Mutex{}

func main() {
    rand.Seed(42)
    snapshotter = flag.String("ss", "devmapper", "snapshotter")

    log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
    flag.Parse()

    setupCloseHandler()
    log.Println("Creating containerd client")
    client, g_err = containerd.New(containerdAddress)
    if g_err != nil {
        log.Fatalf("Failed to start containerd client", g_err)
    }
    log.Println("Created containerd client")

    ctx = namespaces.WithNamespace(context.Background(), namespaceName)

    fcClient, g_err = fcclient.New(containerdTTRPCAddress)
    if g_err != nil {
        log.Fatalf("Failed to start firecracker client", g_err)
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
    log.Printf("Received: %v", in.GetImage())
    image, err := client.Pull(ctx, "docker.io/" + in.GetImage(),
                              containerd.WithPullUnpack,
                              containerd.WithPullSnapshotter(*snapshotter),
                             )
    if err != nil {
        return &pb.Status{Message: "Pulling a VM image failed"}, errors.Wrapf(err, "creating container")
    }

    vmID := in.GetId()
    netID := rand.Intn(2) + 1
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
        _, err1 := fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID})
        if err1 != nil { log.Printf("Attempt to clean up failed...") }
        return &pb.Status{Message: "Failed to start VM"}, errors.Wrap(err, "failed to create the VM")
    }
    container, err := client.NewContainer(
                                          ctx,
                                          in.GetId(),
                                          containerd.WithSnapshotter(*snapshotter),
                                          containerd.WithNewSnapshot(in.GetId(), image),
                                          containerd.WithNewSpec(
                                                                 oci.WithImageConfig(image),
                                                                 firecrackeroci.WithVMID(vmID),
                                                                 firecrackeroci.WithVMNetwork,
                                                                ),
                                          containerd.WithRuntime("aws.firecracker", nil),
                                         )
    if err != nil {
        return &pb.Status{Message: "Failed to start container for the VM" + in.GetId() }, err
    }
    task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
    if err != nil {
        err1 := container.Delete(ctx, containerd.WithSnapshotCleanup)
        if err1 != nil { log.Printf("Attempt to clean up failed...") }
        _, err1 = fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID})
        if err1 != nil { log.Printf("Attempt to clean up failed...") }
        return &pb.Status{Message: "Failed to create the task for the VM" + in.GetId() }, err

    }

    log.Printf("Successfully created task: %s for the container\n", task.ID())
    _, err = task.Wait(ctx)
    if err != nil {
        _, err1 := task.Delete(ctx)
        if err1 != nil { log.Printf("Attempt to clean up failed...") }
        err1 = container.Delete(ctx, containerd.WithSnapshotCleanup)
        if err1 != nil { log.Printf("Attempt to clean up failed...") }
        _, err1 = fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID})
        if err1 != nil { log.Printf("Attempt to clean up failed...") }
        return &pb.Status{Message: "Failed to wait for the task for the VM" + in.GetId() }, err

    }

    log.Println("Completed waiting for the container task")
    if err := task.Start(ctx); err != nil {
        _, err1 := task.Delete(ctx)
        if err1 != nil { log.Printf("Attempt to clean up failed...") }
        err1 = container.Delete(ctx, containerd.WithSnapshotCleanup)
        if err1 != nil { log.Printf("Attempt to clean up failed...") }
        _, err1 = fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID})
        if err1 != nil { log.Printf("Attempt to clean up failed...") }
        return &pb.Status{Message: "Failed to start the task for the VM" + in.GetId() }, err

    }

    log.Println("Successfully started the container task")

    mu.Lock()
    active_vms = append(active_vms, VM{Ctx: ctx, Image: image, Container: container, Task: task, VMID: vmID})
    mu.Unlock()
    //TODO: set up port forwarding to a private IP

    return &pb.Status{Message: "started VM " + in.GetId() }, nil
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

func stopActiveVMs() error {
    var mux sync.Mutex
    var vmGroup sync.WaitGroup
    for _, vm := range active_vms {
        vmGroup.Add(1)
        go func(vm VM) {
            defer vmGroup.Done()
            log.Println("Waiting for the killed task for the VM" + vm.VMID)
            var err error

            if err != nil {
                //return errors.Wrapf(err, "Waiting for the killed task")
            }
            log.Println("Killing the task for the VM" + vm.VMID)
            if err = vm.Task.Kill(vm.Ctx, syscall.SIGKILL); err != nil {
                //return errors.Wrapf(err, "killing task")
            }
            log.Println("Deleting the task for the VM" + vm.VMID)
            _, err = vm.Task.Delete(vm.Ctx)
            if err != nil {
                log.Printf("failed to delete the task of the VM, err: %v\n", err)
                //return err
            }
            err = vm.Container.Delete(vm.Ctx, containerd.WithSnapshotCleanup)
            if err != nil {
                log.Printf("failed to delete the container of the VM, err: %v\n", err)
                //return err
            }

            mux.Lock()
            log.Println("Stopping the VM" + vm.VMID)
            _, err = fcClient.StopVM(vm.Ctx, &proto.StopVMRequest{VMID: vm.VMID})
            if err != nil {
                log.Printf("failed to stop the VM, err: %v\n", err)
                //return err
            }
            mux.Unlock()
            log.Println("unlocked mutex the VM")
        }(vm)
    }
    log.Println("waiting for goroutines")
    vmGroup.Wait()
    log.Println("waiting done")

    log.Println("Closing fcClient")
    fcClient.Close()
    log.Println("Closing containerd client")
    client.Close()
    return nil
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
