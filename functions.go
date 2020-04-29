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
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/semaphore"

	log "github.com/sirupsen/logrus"
	hpb "github.com/ustiugov/fccd-orchestrator/helloworld"
)

//////////////////////////////// FunctionPool type //////////////////////////////////////////

// FuncPool Pool of functions
type FuncPool struct {
	sync.Mutex
	funcMap        map[string]*Function
	saveMemoryMode bool
	servedTh       uint64
	pinnedFuncNum  int
	stats          *Stats
}

// NewFuncPool Initializes a pool of functions. Functions can only be added
// but never removed from the map.
func NewFuncPool(saveMemoryMode bool, servedTh uint64, pinnedFuncNum int) *FuncPool {
	p := new(FuncPool)
	p.funcMap = make(map[string]*Function)
	p.saveMemoryMode = saveMemoryMode
	p.servedTh = servedTh
	p.pinnedFuncNum = pinnedFuncNum
	p.stats = NewStats()

	return p
}

// getFunction Returns a ptr to a function or creates it unless it exists
func (p *FuncPool) getFunction(fID, imageName string) *Function {
	p.Lock()
	defer p.Unlock()

	logger := log.WithFields(log.Fields{"fID": fID, "imageName": imageName})

	_, found := p.funcMap[fID]
	if !found {
		isToPin := true

		fIDint, err := strconv.Atoi(fID)
		log.Debug(fmt.Sprintf("fIDint=%d, err=%v, pinnedFuncNum=%d", fIDint, err, p.pinnedFuncNum))
		if p.saveMemoryMode && err == nil && fIDint > p.pinnedFuncNum {
			isToPin = false
		}

		logger.Debug(fmt.Sprintf("Created function, pinned=%t, shut down after %d requests", isToPin, p.servedTh))
		p.funcMap[fID] = NewFunction(fID, imageName, p.stats, p.servedTh, isToPin)

		if err := p.stats.CreateStats(fID); err != nil {
			logger.Panic("GetFunction: Function exists")
		}
	}

	return p.funcMap[fID]
}

// Serve Service RPC request by triggering the corresponding function.
func (p *FuncPool) Serve(ctx context.Context, fID, imageName, payload string) (*hpb.FwdHelloResp, error) {
	f := p.getFunction(fID, imageName)

	return f.Serve(ctx, fID, imageName, payload)
}

// RemoveInstance Removes instance of the function (blocking)
func (p *FuncPool) RemoveInstance(fID, imageName string) (string, error) {
	f := p.getFunction(fID, imageName)

	return f.RemoveInstance()
}

//////////////////////////////// Function type //////////////////////////////////////////////

// Function type
type Function struct {
	sync.RWMutex
	OnceAddInstance   *sync.Once
	sem               *semaphore.Weighted
	fID               string
	imageName         string
	vmIDList          []string // FIXME: only a single VM per function is supported
	lastInstanceID    int
	isPinnedInMem     bool // if pinned, the orchestrator does not stop/offload it)
	stats             *Stats
	servedTh          uint64
	servedSyncCounter int64
}

// NewFunction Initializes a function
// Note: for numerical fIDs, [0, hotFunctionsNum) and [hotFunctionsNum; hotFunctionsNum+warmFunctionsNum)
// are functions that are pinned in memory (stopping or offloading by the daemon is not allowed)
func NewFunction(fID, imageName string, Stats *Stats, servedTh uint64, isToPin bool) *Function {
	f := new(Function)
	f.fID = fID
	f.imageName = imageName
	f.OnceAddInstance = new(sync.Once)
	f.sem = semaphore.NewWeighted(int64(servedTh))
	f.isPinnedInMem = isToPin
	f.stats = Stats
	f.servedTh = servedTh
	f.servedSyncCounter = int64(servedTh) // cannot use uint64 for the counter due to the overflow

	return f
}

// Serve Service RPC request and response on behalf of a function, spinning
// function instances when necessary.
//
// Synchronization description:
// 1. Function needs to start an instance (with a unique vmID) if there are none: goroutines are synchronized with do.Once
// 2. Function (that is not pinned) can serve only up to servedTh requests (controlled by a WeightedSemaphore)
//    a. The last goroutine needs to trigger the function's instance shutdown, then reset the semaphore,
//       allowing new goroutines to serve their requests.
//    b. The last goroutine is determined by the atomic counter: the goroutine wih syncID==0 shuts down
//       the instance.
//    c. Instance shutdown is performed asynchronously because all instances have unique IDs.
func (f *Function) Serve(ctx context.Context, fID, imageName, reqPayload string) (*hpb.FwdHelloResp, error) {
	syncID := int64(-1) // default is no synchronization

	logger := log.WithFields(log.Fields{"fID": f.fID})

	isColdStart := false

	if !f.isPinnedInMem {
		if err := f.sem.Acquire(context.Background(), 1); err != nil {
			logger.Panic("Failed to acquire semaphore for serving")
		}

		syncID = atomic.AddInt64(&f.servedSyncCounter, -1) // unique number for goroutines acquiring the semaphore
	}

	f.stats.IncServed(f.fID)

	f.OnceAddInstance.Do(
		func() {
			isColdStart = true
			logger.Debug("Function is inactive, starting the instance...")
			f.AddInstance()
		})

	f.RLock()
	resp, err := f.fwdRPC(ctx, reqPayload)
	if err != nil {
		logger.Warn("Function returned error: ", err)
		return &hpb.FwdHelloResp{IsColdStart: isColdStart, Payload: ""}, err
	}
	f.RUnlock()

	if !f.isPinnedInMem && syncID == 0 {
		logger.Debug(fmt.Sprintf("Function has to shut down its instance, served %d requests", f.GetStatServed()))
		f.RemoveInstanceAsync()
		f.ZeroServedStat()
		f.servedSyncCounter = int64(f.servedTh) // reset counter
		f.sem.Release(int64(f.servedTh))
	}

	return &hpb.FwdHelloResp{IsColdStart: isColdStart, Payload: resp.Message}, err
}

func (f *Function) getInstanceVMID() string {
	return f.vmIDList[0] // TODO: load balancing when many instances are supported
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
// Note: this function is called from sync.Once construct
func (f *Function) AddInstance() {
	f.Lock()
	defer f.Unlock()

	logger := log.WithFields(log.Fields{"fID": f.fID})

	logger.Debug("Adding instance")

	vmID := fmt.Sprintf("%s_%d", f.fID, f.lastInstanceID)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	message, _, err := orch.StartVM(ctx, vmID, f.imageName)
	if err != nil {
		log.Panic(message, err)
	}

	f.stats.IncStarted(f.fID)

	f.vmIDList = append(f.vmIDList, vmID)
	f.lastInstanceID++
}

// RemoveInstanceAsync Stops an instance (VM) of the function.
// Note: this function is called from sync.Once construct
func (f *Function) RemoveInstanceAsync() {
	f.Lock()
	defer f.Unlock()

	logger := log.WithFields(log.Fields{"fID": f.fID})

	logger.Debug("Removing instance (async)")

	vmID := f.clearInstanceState()

	go func(vmID string) {
		message, err := orch.StopSingleVM(context.Background(), vmID)
		if err != nil {
			log.Warn(message, err)
		}
	}(vmID)
}

// RemoveInstance Stops an instance (VM) of the function.
func (f *Function) RemoveInstance() (string, error) {
	f.Lock()
	logger := log.WithFields(log.Fields{"fID": f.fID})

	logger.Debug("Removing instance")

	vmID := f.clearInstanceState()
	f.Unlock()

	return orch.StopSingleVM(context.Background(), vmID)
}

func (f *Function) clearInstanceState() (vmID string) {
	vmID, f.vmIDList = f.vmIDList[0], f.vmIDList[1:]

	if len(f.vmIDList) == 0 {
		f.OnceAddInstance = new(sync.Once)
	} else {
		log.Panic("List of function's instance is not empty after stopping an instance!")
	}

	return vmID
}

// GetStatServed Returns the served counter value
func (f *Function) GetStatServed() uint64 {
	return atomic.LoadUint64(&f.stats.statMap[f.fID].served)
}

// ZeroServedStat Zero served counter
func (f *Function) ZeroServedStat() {
	atomic.StoreUint64(&f.stats.statMap[f.fID].served, 0)
}
