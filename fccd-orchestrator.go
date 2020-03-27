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
    _ "fmt"
    _ "io/ioutil"
    "io"
    "log"
    "net"
    "math/rand"
    "os"
    "time"

    "google.golang.org/grpc"
    pb "github.com/ustiugov/fccd-orchestrator/proto"
    hpb "github.com/ustiugov/fccd-orchestrator/helloworld"
    "github.com/ustiugov/fccd-orchestrator/ctrIface"
)

const (
    port = ":3333"
    fwdPort = ":3334"
)

var flog *os.File
var orch *ctrIface.Orchestrator

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
    snapshotter := flag.String("ss", "devmapper", "snapshotter name")
    niNum := flag.Int("ni", 1500, "Number of interfaces allocated")

    if flog, err = os.Create("/tmp/fccd.log"); err != nil {
        panic(err)
    }
    defer flog.Close()

    //log.SetOutput(&myWriter{Writer: flog})
    log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
    flag.Parse()

    orch = ctrIface.NewOrchestrator(*snapshotter, *niNum)

    go fwdServe()

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

type fwdServer struct {
    hpb.UnimplementedFwdGreeterServer
}

func fwdServe() {
    lis, err := net.Listen("tcp", fwdPort)
    if err != nil {
        log.Fatalf("failed to listen: %v", err)
    }
    s := grpc.NewServer()
    hpb.RegisterFwdGreeterServer(s, &fwdServer{})

    log.Println("Listening on port" + fwdPort)
    if err := s.Serve(lis); err != nil {
        log.Fatalf("failed to serve: %v", err)
    }
}

func (s *server) StartVM(ctx context.Context, in *pb.StartVMReq) (*pb.StartVMResp, error) {
    vmID := in.GetId()
    image := in.GetImage()

    message, t_profile, err := orch.StartVM(ctx, vmID, image)
    if err != nil {
        return &pb.StartVMResp{Message: message, Profile: t_profile }, err
    }

    return &pb.StartVMResp{Message: "started VM " + vmID, Profile: t_profile }, nil
}

func (s *server) StopSingleVM(ctx context.Context, in *pb.StopSingleVMReq) (*pb.Status, error) {
    vmID := in.GetId()
    log.Printf("Received stop single VM request for VM %v", vmID)
    if message, err := orch.StopSingleVM(ctx, vmID); err != nil {
        return &pb.Status{Message: message }, err
    }

    return &pb.Status{Message: "Stopped VM"}, nil
}

func (s *server) StopVMs(ctx context.Context, in *pb.StopVMsReq) (*pb.Status, error) {
    log.Println("Received StopVMs request")
    err := orch.StopActiveVMs()
    if err != nil {
        log.Printf("Failed to stop VMs, err: %v\n", err)
        return &pb.Status{Message: "Failed to stop VMs"}, err
    }
    os.Exit(0)
    return &pb.Status{Message: "Stopped VMs"}, nil
}

func (s *fwdServer) FwdHello(ctx context.Context, in *hpb.FwdHelloReq) (*hpb.FwdHelloResp, error) {
    vmID := in.GetId()
    image := in.GetImage()
    payload := in.GetPayload()

    log.Println("Received FwdHello for VM %v, image %v, payload %v", vmID, image, payload)

    isColdStart := false

    if !orch.IsVMActive(vmID) {
        isColdStart = true
        message, _, err := orch.StartVM(ctx, vmID, image) // message, t_profile
        if err != nil {
            log.Println("FWD: " + message)
            // TBD: retry if failed (workaround)
        }
    }

    funcClient, err := orch.GetFuncClientByID(vmID)
    if err != nil {
        return &hpb.FwdHelloResp{IsColdStart: isColdStart, Payload: ""}, err
    }

    ctx, cancel := context.WithTimeout(context.Background(), time.Second * 10)
    defer cancel()
    resp, err := funcClient.SayHello(ctx, &hpb.HelloRequest{Name: payload})
    if err != nil {
        return &hpb.FwdHelloResp{IsColdStart: isColdStart, Payload: resp.Message}, err
    }

    // TBD: inject cold starts here

    return &hpb.FwdHelloResp{IsColdStart: isColdStart, Payload: resp.Message}, nil
}

