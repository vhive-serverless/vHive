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
	"math/rand"
	"net"
	"os"
	"time"

	ctrdlog "github.com/containerd/containerd/log"
	log "github.com/sirupsen/logrus"

	ctriface "github.com/ustiugov/fccd-orchestrator/ctriface"
	hpb "github.com/ustiugov/fccd-orchestrator/helloworld"
	pb "github.com/ustiugov/fccd-orchestrator/proto"
	"google.golang.org/grpc"
)

const (
	port         = ":3333"
	fwdPort      = ":3334"
	backoffDelay = 10 * time.Millisecond
)

var flog *os.File
var orch *ctriface.Orchestrator
var funcPool *FuncPool

/*
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
*/

func main() {
	var err error

	rand.Seed(42)
	snapshotter := flag.String("ss", "devmapper", "snapshotter name")
	niNum := flag.Int("ni", 1500, "Number of interfaces allocated")
	debug := flag.Bool("dbg", false, "Enable debug logging")

	if flog, err = os.Create("/tmp/fccd.log"); err != nil {
		panic(err)
	}
	defer flog.Close()

	//log.SetOutput(&myWriter{Writer: flog})
	//	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)
	flag.Parse()

	if *debug == true {
		log.SetLevel(log.DebugLevel)
		log.Debug("Debug logging is enabled")
	} else {
		log.SetLevel(log.InfoLevel)
	}

	orch = ctriface.NewOrchestrator(*snapshotter, *niNum)

	funcPool = NewFuncPool()

	go orchServe()
	fwdServe()
}

type server struct {
	pb.UnimplementedOrchestratorServer
}

type fwdServer struct {
	hpb.UnimplementedFwdGreeterServer
}

func orchServe() {
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
	log.WithFields(log.Fields{"vmID": vmID, "image": imageName}).Info("Received StartVM")

	message, tProfile, err := orch.StartVM(ctx, vmID, imageName)
	if err != nil {
		return &pb.StartVMResp{Message: message, Profile: tProfile}, err
	}

	return &pb.StartVMResp{Message: "started VM " + vmID, Profile: tProfile}, nil
}

func (s *server) StopSingleVM(ctx context.Context, in *pb.StopSingleVMReq) (*pb.Status, error) {
	vmID := in.GetId()
	log.WithFields(log.Fields{"vmID": vmID}).Info("Received StopVM")
	message, err := orch.StopSingleVM(ctx, vmID)
	if err != nil {
		log.Warn(message, err)
	}

	return &pb.Status{Message: message}, err
}

// Note: this function is to be used only before tearing down the whole orchestrator
func (s *server) StopVMs(ctx context.Context, in *pb.StopVMsReq) (*pb.Status, error) {
	log.Info("Received StopVMs")
	err := orch.StopActiveVMs()
	if err != nil {
		log.Printf("Failed to stop VMs, err: %v\n", err)
		return &pb.Status{Message: "Failed to stop VMs"}, err
	}
	os.Exit(0)
	return &pb.Status{Message: "Stopped VMs"}, nil
}

func (s *fwdServer) FwdHello(ctx context.Context, in *hpb.FwdHelloReq) (*hpb.FwdHelloResp, error) {
	fID := in.GetId()
	imageName := in.GetImage()
	payload := in.GetPayload()

	logger := log.WithFields(log.Fields{"vmID": fID, "image": imageName, "payload": payload})
	logger.Debug("Received FwdHelloVM")

	isColdStart := false

	fun := funcPool.GetFunction(fID, imageName)

	if !fun.IsActive() {
		ctxAddInst, cancelAddInst := context.WithTimeout(context.Background(), time.Second*30)
		defer cancelAddInst()

		fun.AddInstance(ctxAddInst)
	}

	resp, err := fun.FwdRPC(ctx, payload)
	if err != nil {
		logger.Warn("Function returned error: ", err)
		return &hpb.FwdHelloResp{IsColdStart: isColdStart, Payload: ""}, err
	}

	return &hpb.FwdHelloResp{IsColdStart: isColdStart, Payload: resp.Message}, nil
}

/*
func handleColdStart(vmID string, imageName string) (message string, err error) {
	logger := log.WithFields(log.Fields{"vmID": vmID})

	if orch.IsVMStateStarting(vmID) {
		for i := 0; i < 1000; i++ {
			time.Sleep(backoffDelay)
			if orch.IsVMStateActive(vmID) {
				logger.Debug(fmt.Sprintf("Waiting for the VM to start (%d)...", i))
				break
			} else if i == 999 {
				logger.Warn("Failed to wait for the VM to start")
				message = "Failed to wait for the VM to start"
				err = errors.New("Failed to wait for the VM to start")
			}
		}
	} else if orch.IsVMStateDeactivating(vmID) || orch.IsVMOff(vmID) {
		for !orch.IsVMOff(vmID) {
			logger.Debug("Waiting for the VM to shut down...")
			time.Sleep(backoffDelay)
		}
		// TODO: call into goroutines to trigger startVM / prefetch
		logger.Debug("VM does not exist for FwdHello")
		message, _, err = orch.StartVM(context.Background(), vmID, imageName)
		if err != nil {
			logger.Debug(message, err)
		}
		return message, err
	} else {
		logger.Panic("Cold start handling failed.")
	}
	return message, err
}
*/
