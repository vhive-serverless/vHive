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

package main

import (
    "context"
    "flag"
    "fmt"
    _ "io/ioutil"
    "io"
    "log"
    "net"
    "syscall"
    "time"
    "math/rand"
    "strconv"

    "google.golang.org/grpc"
    pb "github.com/ustiugov/fccd-orchestrator/proto"
    "github.com/ustiugov/fccd-orchestrator/misc"

    "os"
    "os/signal"
    "sync"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"

//    "github.com/ustiugov/skv"
)

const (
    port = ":3333"
)

var flog *os.File
//var store *skv.KVStore //TODO: stop individual VMs

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
    snapshotter = flag.String("ss", "devmapper", "snapshotter name")
    niNum := flag.Int("ni", 1500, "Number of interfaces allocated")

    if flog, err = os.Create("/tmp/fccd.log"); err != nil {
        panic(err)
    }
    defer flog.Close()

    //log.SetOutput(&myWriter{Writer: flog})
    log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
    flag.Parse()

    o := NewOrchestrator(*snapshotter, niNum)

//    store, err = skv.Open("/var/lib/fccd-orchestrator/vms.db")
//    if err != nil { log.Fatalf("Failed to open db file", err) }

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

func (s *server) StartVM(ctx_ context.Context, in *pb.StartVMReq) (*pb.StartVMResp, error) {
    vmID := in.GetId()
    image := in.GetImage()

    message, t_profile, err := ctrIface.StartVM(vmID, image, ni)
    if err != nil {
        return &pb.StartVMResp{Message: message, Profile: t_profile }, err
    }

    return &pb.StartVMResp{Message: "started VM " + vmID, Profile: t_profile }, nil
}

func (s *server) StopSingleVM(ctx_ context.Context, in *pb.StopSingleVMReq) (*pb.Status, error) {
    vmID := in.GetId()
    log.Printf("Received stop single VM request for VM %v", vmID)
    ctx := namespaces.WithNamespace(context.Background(), namespaceName)
    if vm, is_present := active_vms[vmID]; is_present {
        ctx, _ = context.WithDeadline(ctx, time.Now().Add(time.Duration(60) * time.Second))
        if err := vm.Task.Kill(ctx, syscall.SIGKILL); err != nil {
            log.Printf("Failed to kill the task, err: %v\n", err)
            return &pb.Status{Message: "Killing task of VM " + vmID + " failed"}, err
        }
        if _, err := vm.Task.Wait(ctx); err != nil {
            log.Printf("Failed to wait for the task to be killed, err: %v\n", err)
            return &pb.Status{Message: "Killing (waiting) task of VM " + vmID + " failed"}, err
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
        ctx := namespaces.WithNamespace(context.Background(), namespaceName)
        if _, err := fcClient.StopVM(ctx, &proto.StopVMRequest{VMID: vmID}); err != nil {
            log.Printf("failed to stop the VM, err: %v\n", err)
            return &pb.Status{Message: "Stopping VM " + vmID + " failed"}, err
        }

        niList = append(niList, vm.Ni)
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
        go func(vmID string, vm misc.VM) {
            defer vmGroup.Done()
            ctx := namespaces.WithNamespace(context.Background(), namespaceName)
            ctx, _ = context.WithDeadline(ctx, time.Now().Add(time.Duration(300) * time.Second))
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

