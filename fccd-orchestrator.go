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
	"math/rand"
	"net"
	"os"
	"runtime"

	ctrdlog "github.com/containerd/containerd/log"
	log "github.com/sirupsen/logrus"

	ctriface "github.com/ustiugov/fccd-orchestrator/ctriface"
	hpb "github.com/ustiugov/fccd-orchestrator/helloworld"
	pb "github.com/ustiugov/fccd-orchestrator/proto"
	"google.golang.org/grpc"
)

const (
	port    = ":3333"
	fwdPort = ":3334"
)

var (
	flog     *os.File
	orch     *ctriface.Orchestrator
	funcPool *FuncPool

	isSaveMemory       *bool
	isSnapshotsEnabled *bool
	isUPFEnabled       *bool
	servedThreshold    *uint64
	pinnedFuncNum      *int
)

func main() {
	var err error
	runtime.GOMAXPROCS(16)

	rand.Seed(42)
	snapshotter := flag.String("ss", "devmapper", "snapshotter name")
	debug := flag.Bool("dbg", false, "Enable debug logging")

	isSaveMemory = flag.Bool("ms", false, "Enable memory saving")
	isSnapshotsEnabled = flag.Bool("snapshots", false, "Enable snapshots")
	isUPFEnabled = flag.Bool("upf", false, "Enable UPF")
	servedThreshold = flag.Uint64("st", 1000*1000, "Functions serves X RPCs before it shuts down (if saveMemory=true)")
	pinnedFuncNum = flag.Int("hn", 0, "Number of functions pinned in memory (IDs from 0 to X)")

	if flog, err = os.Create("/tmp/fccd.log"); err != nil {
		panic(err)
	}
	defer flog.Close()

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)
	flag.Parse()

	if *debug {
		log.SetLevel(log.DebugLevel)
		log.Debug("Debug logging is enabled")
	} else {
		log.SetLevel(log.InfoLevel)
	}

	if *isSaveMemory {
		log.Info(fmt.Sprintf("Creating orchestrator for pinned=%d functions", *pinnedFuncNum))
	}

	testModeOn := false

	orch = ctriface.NewOrchestrator(
		*snapshotter,
		ctriface.WithTestModeOn(testModeOn),
		ctriface.WithSnapshots(*isSnapshotsEnabled),
		ctriface.WithUPFEnabled(*isUPFEnabled),
	)

	funcPool = NewFuncPool(*isSaveMemory, *servedThreshold, *pinnedFuncNum, testModeOn)

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

// StartVM, StopSingleVM and StopVMs are legacy functions that manage functions and VMs
// Should be used only to bootstrap an experiment (e.g., quick parallel start of many functions)
func (s *server) StartVM(ctx context.Context, in *pb.StartVMReq) (*pb.StartVMResp, error) {
	fID := in.GetId()
	imageName := in.GetImage()
	log.WithFields(log.Fields{"fID": fID, "image": imageName}).Info("Received direct StartVM")

	message, err := funcPool.AddInstance(fID, imageName)
	tProfile := "not supported anymore"
	//message, tProfile, err := orch.StartVM(ctx, fID, imageName)
	if err != nil { // does not return error
		return &pb.StartVMResp{Message: message, Profile: tProfile}, err
	}

	return &pb.StartVMResp{Message: "started VM instance for a function " + fID, Profile: tProfile}, nil
}

func (s *server) StopSingleVM(ctx context.Context, in *pb.StopSingleVMReq) (*pb.Status, error) {
	fID := in.GetId()
	isSync := true
	log.WithFields(log.Fields{"fID": fID}).Info("Received direct StopVM")
	message, err := funcPool.RemoveInstance(fID, "bogus imageName", isSync)
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

	logger := log.WithFields(log.Fields{"fID": fID, "image": imageName, "payload": payload})
	logger.Debug("Received FwdHelloVM")

	resp, _, err := funcPool.Serve(ctx, fID, imageName, payload)
	return resp, err
}
