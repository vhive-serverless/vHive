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
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	hpb "github.com/ustiugov/fccd-orchestrator/helloworld"
)

//////////////////////////////// FunctionPool type //////////////////////////////////////////

// FuncPool Pool of functions
type FuncPool struct {
	sync.Mutex
	funcMap   map[string]*Function
	coldStats *ColdStats
}

// NewFuncPool Initializes a pool of functions. Functions can only be added
// but never removed from the map.
func NewFuncPool() *FuncPool {
	p := new(FuncPool)
	p.funcMap = make(map[string]*Function)
	p.coldStats = NewColdStats()

	return p
}

// GetFunction Returns a ptr to a function or creates it unless it exists
func (p *FuncPool) GetFunction(fID, imageName string, isToPin bool) *Function {
	p.Lock()
	defer p.Unlock()

	_, found := p.funcMap[fID]
	if !found {
		p.funcMap[fID] = NewFunction(fID, imageName, p.coldStats, isToPin)
		if err := p.coldStats.CreateStats(fID); err != nil {
			log.WithFields(log.Fields{"fID": fID}).Panic("GetFunction: Function exists")
		}
	}

	return p.funcMap[fID]
}

//////////////////////////////// Function type //////////////////////////////////////////////

// Function type
type Function struct {
	sync.RWMutex
	Once           *sync.Once
	fID            string
	imageName      string
	vmIDList       []string // FIXME: only a single VM per function is supported
	lastInstanceID int
	isActive       bool
	isPinnedInMem  bool // if pinned, the orchestrator does not stop/offload it)
	coldStats      *ColdStats
}

// NewFunction Initializes a function
// Note: for numerical fIDs, [0, hotFunctionsNum) and [hotFunctionsNum; hotFunctionsNum+warmFunctionsNum)
// are functions that are pinned in memory (stopping or offloading by the daemon is not allowed)
func NewFunction(fID, imageName string, coldStats *ColdStats, isToPin bool) *Function {
	f := new(Function)
	f.fID = fID
	f.imageName = imageName
	f.Once = new(sync.Once)
	f.isPinnedInMem = false
	f.coldStats = coldStats
	f.isPinnedInMem = isToPin

	return f
}

// IsActive Returns if function is active and ready to serve requests.
func (f *Function) IsActive() bool {
	f.RLock()
	defer f.RUnlock()

	return f.isActive
}

func (f *Function) getInstanceVMID() string {
	return f.vmIDList[0] // TODO: load balancing when many instances are supported
}

// Serve Service RPC request and response on behalf of a function, spinning
// function instances when necessary.
func (f *Function) Serve(ctx context.Context, imageName, reqPayload string) (*hpb.FwdHelloResp, error) {
	logger := log.WithFields(log.Fields{"fID": f.fID})

	isColdStart := false

	if !f.IsActive() {
		isColdStart = true
		onceBody := func() {
			f.AddInstance()
		}
		f.Once.Do(onceBody)
	}

	resp, err := f.fwdRPC(ctx, reqPayload)
	if err != nil {
		logger.Warn("Function returned error: ", err)
		return &hpb.FwdHelloResp{IsColdStart: isColdStart, Payload: ""}, err
	}

	if !f.isPinnedInMem {
		f.coldStats.IncServed(f.fID)
	}

	return &hpb.FwdHelloResp{IsColdStart: isColdStart, Payload: resp.Message}, err
}

// FwdRPC Forward the RPC to an instance, then forwards the response back.
func (f *Function) fwdRPC(ctx context.Context, reqPayload string) (*hpb.HelloReply, error) {
	f.RLock()
	defer f.RUnlock()

	logger := log.WithFields(log.Fields{"fID": f.fID})

	funcClientPtr, err := orch.GetFuncClient(f.getInstanceVMID())
	if err != nil {
		return &hpb.HelloReply{Message: "Failed to get function client"}, err
	}

	funcClient := *funcClientPtr

	logger.Debug("FwdRPC: Forwarding RPC to function instance")
	resp, err := funcClient.SayHello(ctx, &hpb.HelloRequest{Name: reqPayload})
	logger.Debug("FwdRPC: Received a response from the  function instance")

	return resp, err
}

// AddInstance Starts a VM, waits till it is ready.
func (f *Function) AddInstance() {
	f.Lock()
	defer f.Unlock()

	vmID := fmt.Sprintf("%s_%d", f.fID, f.lastInstanceID)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	message, _, err := orch.StartVM(ctx, vmID, f.imageName)
	if err != nil {
		log.Panic(message, err)
	}

	if !f.isPinnedInMem {
		f.coldStats.IncStarted(f.fID)
	}

	f.vmIDList = append(f.vmIDList, vmID)
	f.lastInstanceID++

	f.isActive = true
}

// RemoveInstance Stops an instance (VM) of the function.
func (f *Function) RemoveInstance() (string, error) {
	f.Lock()
	var vmID string
	vmID, f.vmIDList = f.vmIDList[0], f.vmIDList[1:]

	if len(f.vmIDList) == 0 {
		f.isActive = false
		f.Once = new(sync.Once)
	} else {
		panic("List of function's instance is not empty after stopping an instance!")
	}

	f.Unlock()

	message, err := orch.StopSingleVM(context.Background(), vmID)
	if err != nil {
		log.Warn(message, err)
	}

	return message, err
}
