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

    _ "github.com/pkg/errors"

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
    imageName := in.GetImage()
    log.Printf("Received StartVM for VM %v, image %v", vmID, imageName)

    message, t_profile, err := orch.StartVM(ctx, vmID, imageName)
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
    //imageName := in.GetImage()
    payload := in.GetPayload()

    //log.Printf("Received FwdHello for VM %v, image %v, payload %v", vmID, imageName, payload)

    isColdStart := false
    // FIXME: DEADLINES ARE TOTALMESS BELOW
//    if orch.IsVMActive(vmID) == false {
//        // FIXME: remove the return below, handle restart
//        isColdStart = true
//        log.Printf("VM does not exist for FwdHello: VM %v, image %v, payload %v; requesting StartVM", vmID, imageName, payload)
//        ctx_start, _ := context.WithDeadline(context.Background(), time.Now().Add(time.Duration(60) * time.Second))
//        _, _, err := orch.StartVM(ctx_start, vmID, imageName) // message, t_profile
//        if ctx.Err() == context.Canceled { // Original RPC cancelled or timed out
//            return &hpb.FwdHelloResp{IsColdStart: isColdStart, Payload: ""}, errors.New("Cancelled after cold start")
//        } else if err != nil {
//            //log.Printf("FWD service is attempting a restart upon failure: %v %v", message, err)
//            return &hpb.FwdHelloResp{IsColdStart: isColdStart, Payload: ""}, errors.Wrapf(err, "StartVM failed")
//            if message, err := orch.StopSingleVM(ctx, vmID); err != nil {
//                return &hpb.FwdHelloResp{IsColdStart: isColdStart, Payload: ""}, errors.Wrapf(err, message+" Restart: Stop failure")
//            }
//            if message, _, err := orch.StartVM(ctx, vmID, imageName); err != nil {
//                return &hpb.FwdHelloResp{IsColdStart: isColdStart, Payload: ""}, errors.Wrapf(err, message+" Restart failed")
//            }
//        }
//    }

    funcClient, err := orch.GetFuncClientByID(vmID)
    if err != nil {
        return &hpb.FwdHelloResp{IsColdStart: isColdStart, Payload: ""}, err
    }

    ctx_fwd, _ := context.WithTimeout(context.Background(), time.Millisecond * 100)
//    defer cancel() here  _
    resp, err := funcClient.SayHello(ctx_fwd, &hpb.HelloRequest{Name: payload})
    if err != nil {
        //log.Printf("Function returned error: vmID=%, err=%v", vmID, err)
        return &hpb.FwdHelloResp{IsColdStart: isColdStart, Payload: ""}, err
    }
    // TBD: inject cold starts here
//    if rand.Intn(2) > 0 {
//        log.Printf("Want to stop VM %v", vmID)
//        if message, err := orch.StopSingleVM(ctx, vmID); err != nil {
//            return &hpb.FwdHelloResp{IsColdStart: isColdStart, Payload: message}, err
//        }
//        time.Sleep(5 * time.Second)
//    }
    return &hpb.FwdHelloResp{IsColdStart: isColdStart, Payload: resp.Message}, nil
}

