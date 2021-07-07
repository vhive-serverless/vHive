// MIT License
//
// Copyright (c) 2020 Dmitrii Ustiugov, Plamen Petrov and EASE lab
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
	"math/rand"
	"net"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sync/semaphore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	hpb "github.com/ease-lab/vhive/examples/protobuf/helloworld"
	"github.com/ease-lab/vhive/metrics"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

var isTestMode bool // set with a call to NewFuncPool

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
func NewFuncPool(saveMemoryMode bool, servedTh uint64, pinnedFuncNum int, testModeOn bool) *FuncPool {
	p := new(FuncPool)
	p.funcMap = make(map[string]*Function)
	p.saveMemoryMode = saveMemoryMode
	p.servedTh = servedTh
	p.pinnedFuncNum = pinnedFuncNum
	p.stats = NewStats()

	if !testModeOn {
		heartbeat := time.NewTicker(60 * time.Second)

		go func() {
			for {
				<-heartbeat.C
				log.Info("FuncPool heartbeat: ", p.stats.SprintStats())
			}
		}()
	}

	isTestMode = testModeOn

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
		log.Debugf("fIDint=%d, err=%v, pinnedFuncNum=%d", fIDint, err, p.pinnedFuncNum)
		if p.saveMemoryMode && err == nil && fIDint > p.pinnedFuncNum {
			isToPin = false
		}

		logger.Debugf("Created function, pinned=%t, shut down after %d requests", isToPin, p.servedTh)
		p.funcMap[fID] = NewFunction(fID, imageName, p.stats, p.servedTh, isToPin)

		if err := p.stats.CreateStats(fID); err != nil {
			logger.Panic("GetFunction: Function exists")
		}
	}

	return p.funcMap[fID]
}

// Serve Service RPC request by triggering the corresponding function.
func (p *FuncPool) Serve(ctx context.Context, fID, imageName, payload string) (*hpb.FwdHelloResp, *metrics.Metric, error) {
	f := p.getFunction(fID, imageName)

	return f.Serve(ctx, fID, imageName, payload)
}

// AddInstance Adds instance of the function
func (p *FuncPool) AddInstance(fID, imageName string) (string, error) {
	f := p.getFunction(fID, imageName)

	logger := log.WithFields(log.Fields{"fID": f.fID})

	f.OnceAddInstance.Do(
		func() {
			logger.Debug("Function is inactive, starting the instance...")
			f.AddInstance()
		})

	return "Instance started", nil
}

// RemoveInstance Removes instance of the function (blocking)
func (p *FuncPool) RemoveInstance(fID, imageName string, isSync bool) (string, error) {
	f := p.getFunction(fID, imageName)

	return f.RemoveInstance(isSync)
}

// DumpUPFPageStats Dumps the memory manager's stats for a function about the number of
// the unique pages and the number of the pages that are reused across invocations
func (p *FuncPool) DumpUPFPageStats(fID, imageName, functionName, metricsOutFilePath string) error {
	f := p.getFunction(fID, imageName)

	return f.DumpUPFPageStats(functionName, metricsOutFilePath)
}

// DumpUPFLatencyStats Dumps the memory manager's stats for a function about number of unique/reused pages
func (p *FuncPool) DumpUPFLatencyStats(fID, imageName, functionName, latencyOutFilePath string) error {
	f := p.getFunction(fID, imageName)

	return f.DumpUPFLatencyStats(functionName, latencyOutFilePath)
}

//////////////////////////////// Function type //////////////////////////////////////////////

// Function type
type Function struct {
	sync.RWMutex
	OnceAddInstance        *sync.Once
	fID                    string
	imageName              string
	vmID                   string
	lastInstanceID         int
	isPinnedInMem          bool // if pinned, the orchestrator does not stop/offload it)
	stats                  *Stats
	servedTh               uint64
	sem                    *semaphore.Weighted
	servedSyncCounter      int64
	isSnapshotReady        bool // if ready, the orchestrator should load the instance rather than creating it
	OnceCreateSnapInstance *sync.Once
	funcClient             *hpb.GreeterClient
	conn                   *grpc.ClientConn
	guestIP                string
}

// NewFunction Initializes a function
// Note: for numerical fIDs, [0, hotFunctionsNum) and [hotFunctionsNum; hotFunctionsNum+warmFunctionsNum)
// are functions that are pinned in memory (stopping or offloading by the daemon is not allowed)
func NewFunction(fID, imageName string, Stats *Stats, servedTh uint64, isToPin bool) *Function {
	f := new(Function)
	f.fID = fID
	f.imageName = imageName
	f.OnceAddInstance = new(sync.Once)
	f.isPinnedInMem = isToPin
	f.stats = Stats
	f.OnceCreateSnapInstance = new(sync.Once)

	// Normal distribution with stddev=servedTh/2, mean=servedTh
	thresh := int64(rand.NormFloat64()*float64(servedTh/2) + float64(servedTh))
	if thresh <= 0 {
		thresh = int64(servedTh)
	}
	if isTestMode && servedTh == 40 { // 40 is used in tests
		thresh = 40
	}

	f.servedTh = uint64(thresh)
	f.sem = semaphore.NewWeighted(int64(f.servedTh))
	f.servedSyncCounter = int64(f.servedTh) // cannot use uint64 for the counter due to the overflow

	log.WithFields(
		log.Fields{
			"fID":      f.fID,
			"image":    f.imageName,
			"isPinned": f.isPinnedInMem,
			"servedTh": f.servedTh,
		},
	).Info("New function added")

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
//    b. The last goroutine is determined by the atomic counter: the goroutine with syncID==0 shuts down
//       the instance.
//    c. Instance shutdown is performed asynchronously because all instances have unique IDs.
func (f *Function) Serve(ctx context.Context, fID, imageName, reqPayload string) (*hpb.FwdHelloResp, *metrics.Metric, error) {
	var (
		serveMetric *metrics.Metric = metrics.NewMetric()
		tStart      time.Time
		syncID      int64 = -1 // default is no synchronization
		isColdStart bool  = false
	)

	logger := log.WithFields(log.Fields{"fID": f.fID})

	if !f.isPinnedInMem {
		if err := f.sem.Acquire(context.Background(), 1); err != nil {
			logger.Panic("Failed to acquire semaphore for serving")
		}

		syncID = atomic.AddInt64(&f.servedSyncCounter, -1) // unique number for goroutines acquiring the semaphore
	}

	f.stats.IncServed(f.fID)

	f.OnceAddInstance.Do(
		func() {
			var metr *metrics.Metric
			isColdStart = true
			logger.Debug("Function is inactive, starting the instance...")
			tStart = time.Now()
			metr = f.AddInstance()
			serveMetric.MetricMap[metrics.AddInstance] = metrics.ToUS(time.Since(tStart))

			if metr != nil {
				for k, v := range metr.MetricMap {
					serveMetric.MetricMap[k] = v
				}
			}
		})

	f.RLock()

	// FIXME: keep a strict deadline for forwarding RPCs to a warm function
	// Eventually, it needs to be RPC-dependent and probably client-defined
	ctxFwd, cancel := context.WithDeadline(context.Background(), time.Now().Add(20*time.Second))
	defer cancel()

	tStart = time.Now()
	resp, err := f.fwdRPC(ctxFwd, reqPayload)
	serveMetric.MetricMap[metrics.FuncInvocation] = metrics.ToUS(time.Since(tStart))

	if err != nil && ctxFwd.Err() == context.Canceled {
		// context deadline exceeded
		f.RUnlock()
		return &hpb.FwdHelloResp{IsColdStart: isColdStart, Payload: ""}, serveMetric, err
	} else if err != nil {
		if e, ok := status.FromError(err); ok {
			switch e.Code() {
			case codes.DeadlineExceeded:
				// deadline exceeded
				f.RUnlock()
				return &hpb.FwdHelloResp{IsColdStart: isColdStart, Payload: ""}, serveMetric, err
			default:
				logger.Warn("Function returned error: ", err)
				f.RUnlock()
				return &hpb.FwdHelloResp{IsColdStart: isColdStart, Payload: ""}, serveMetric, err
			}
		} else {
			logger.Panic("Not able to parse error returned ", err)
		}
	}

	if orch.GetSnapshotsEnabled() {
		f.OnceCreateSnapInstance.Do(
			func() {
				logger.Debug("First time offloading, need to create a snapshot first")
				f.CreateInstanceSnapshot()
				f.isSnapshotReady = true
			})
	}

	f.RUnlock()

	if !f.isPinnedInMem && syncID == 0 {
		logger.Debugf("Function has to shut down its instance, served %d requests", f.GetStatServed())
		tStart = time.Now()
		if _, err := f.RemoveInstance(false); err != nil {
			logger.Panic("Failed to remove instance after servedTh expired", err)
		}
		serveMetric.MetricMap[metrics.RetireOld] = metrics.ToUS(time.Since(tStart))
		f.ZeroServedStat()
		f.servedSyncCounter = int64(f.servedTh) // reset counter
		f.sem.Release(int64(f.servedTh))
	}

	return &hpb.FwdHelloResp{IsColdStart: isColdStart, Payload: resp.Message}, serveMetric, err
}

// FwdRPC Forward the RPC to an instance, then forwards the response back.
func (f *Function) fwdRPC(ctx context.Context, reqPayload string) (*hpb.HelloReply, error) {
	f.RLock()
	defer f.RUnlock()

	logger := log.WithFields(log.Fields{"fID": f.fID})

	funcClient := *f.funcClient

	logger.Debug("FwdRPC: Forwarding RPC to function instance")
	resp, err := funcClient.SayHello(ctx, &hpb.HelloRequest{Name: reqPayload})
	logger.Debug("FwdRPC: Received a response from the  function instance")

	return resp, err
}

// AddInstance Starts a VM, waits till it is ready.
// Note: this function is called from sync.Once construct
func (f *Function) AddInstance() *metrics.Metric {
	f.Lock()
	defer f.Unlock()

	logger := log.WithFields(log.Fields{"fID": f.fID})

	logger.Debug("Adding instance")

	var metr *metrics.Metric = nil

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	if f.isSnapshotReady {
		metr = f.LoadInstance()
	} else {
		resp, _, err := orch.StartVM(ctx, f.getVMID(), f.imageName)
		if err != nil {
			log.Panic(err)
		}
		f.guestIP = resp.GuestIP
		f.vmID = f.getVMID()
		f.lastInstanceID++
	}

	tStart := time.Now()
	funcClient, err := f.getFuncClient()
	if metr != nil {
		metr.MetricMap[metrics.ConnectFuncClient] = metrics.ToUS(time.Since(tStart))
	}
	if err != nil {
		logger.Panic("Failed to acquire func client")
	}
	f.funcClient = &funcClient

	f.stats.IncStarted(f.fID)

	return metr
}

// RemoveInstanceAsync Stops an instance (VM) of the function.
func (f *Function) RemoveInstanceAsync() {
	logger := log.WithFields(log.Fields{"fID": f.fID})

	logger.Debug("Removing instance (async)")

	go func() {
		err := orch.StopSingleVM(context.Background(), f.vmID)
		if err != nil {
			log.Warn(err)
		}
	}()
}

// RemoveInstance Stops an instance (VM) of the function.
func (f *Function) RemoveInstance(isSync bool) (string, error) {
	f.Lock()
	defer f.Unlock()

	logger := log.WithFields(log.Fields{"fID": f.fID, "isSync": isSync})

	logger.Debug("Removing instance")

	var (
		r   string
		err error
	)

	f.OnceAddInstance = new(sync.Once)

	if orch.GetSnapshotsEnabled() {
		f.OffloadInstance()
		r = "Successfully offloaded instance " + f.vmID
	} else {
		if isSync {
			err = orch.StopSingleVM(context.Background(), f.vmID)
		} else {
			f.RemoveInstanceAsync()
			r = "Successfully removed (async) instance " + f.vmID
		}
	}

	return r, err
}

// DumpUPFPageStats Dumps the memory manager's stats about the number of
// the unique pages and the number of the pages that are reused across invocations
func (f *Function) DumpUPFPageStats(functionName, metricsOutFilePath string) error {
	return orch.DumpUPFPageStats(f.vmID, functionName, metricsOutFilePath)
}

// DumpUPFLatencyStats Dumps the memory manager's latency stats
func (f *Function) DumpUPFLatencyStats(functionName, latencyOutFilePath string) error {
	return orch.DumpUPFLatencyStats(f.vmID, functionName, latencyOutFilePath)
}

// CreateInstanceSnapshot Creates a snapshot of the instance
func (f *Function) CreateInstanceSnapshot() {
	logger := log.WithFields(log.Fields{"fID": f.fID})

	logger.Debug("Creating instance snapshot")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	err := orch.PauseVM(ctx, f.vmID)
	if err != nil {
		log.Panic(err)
	}

	err = orch.CreateSnapshot(ctx, f.vmID)
	if err != nil {
		log.Panic(err)
	}

	_, err = orch.ResumeVM(ctx, f.vmID)
	if err != nil {
		log.Panic(err)
	}
}

// OffloadInstance Offloads the instance
func (f *Function) OffloadInstance() {
	logger := log.WithFields(log.Fields{"fID": f.fID})

	logger.Debug("Offloading instance")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()

	err := orch.Offload(ctx, f.vmID)
	if err != nil {
		log.Panic(err)
	}
	f.conn.Close()
}

// LoadInstance Loads a new instance of the function from its snapshot and resumes it
// The tap, the shim and the vmID remain the same
func (f *Function) LoadInstance() *metrics.Metric {
	logger := log.WithFields(log.Fields{"fID": f.fID})

	logger.Debug("Loading instance")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*60)
	defer cancel()

	loadMetr, err := orch.LoadSnapshot(ctx, f.vmID)
	if err != nil {
		log.Panic(err)
	}

	resumeMetr, err := orch.ResumeVM(ctx, f.vmID)
	if err != nil {
		log.Panic(err)
	}

	for k, v := range resumeMetr.MetricMap {
		loadMetr.MetricMap[k] = v
	}

	return loadMetr
}

// GetStatServed Returns the served counter value
func (f *Function) GetStatServed() uint64 {
	return atomic.LoadUint64(&f.stats.statMap[f.fID].served)
}

// ZeroServedStat Zero served counter
func (f *Function) ZeroServedStat() {
	atomic.StoreUint64(&f.stats.statMap[f.fID].served, 0)
}

// getVMID Creates the vmID for the function
func (f *Function) getVMID() string {
	return fmt.Sprintf("%s-%d", f.fID, f.lastInstanceID)
}

func (f *Function) getFuncClient() (hpb.GreeterClient, error) {
	backoffConfig := backoff.DefaultConfig
	backoffConfig.MaxDelay = 5 * time.Second
	connParams := grpc.ConnectParams{
		Backoff: backoffConfig,
	}

	gopts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithInsecure(),
		grpc.FailOnNonTempDialError(true),
		grpc.WithConnectParams(connParams),
		grpc.WithContextDialer(contextDialer),
	}

	//  This timeout must be large enough for all functions to start up (e.g., ML training takes few seconds)
	ctxx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctxx, f.guestIP+":50051", gopts...)
	f.conn = conn
	if err != nil {
		return nil, err
	}
	return hpb.NewGreeterClient(conn), nil
}

func contextDialer(ctx context.Context, address string) (net.Conn, error) {
	if deadline, ok := ctx.Deadline(); ok {
		return timeoutDialer(address, time.Until(deadline))
	}
	return timeoutDialer(address, 0)
}

func isConnRefused(err error) bool {
	if err != nil {
		if nerr, ok := err.(*net.OpError); ok {
			if serr, ok := nerr.Err.(*os.SyscallError); ok {
				if serr.Err == syscall.ECONNREFUSED {
					return true
				}
			}
		}
	}
	return false
}

type dialResult struct {
	c   net.Conn
	err error
}

func timeoutDialer(address string, timeout time.Duration) (net.Conn, error) {
	var (
		stopC = make(chan struct{})
		synC  = make(chan *dialResult)
	)
	go func() {
		defer close(synC)
		for {
			select {
			case <-stopC:
				return
			default:
				c, err := net.DialTimeout("tcp", address, timeout)
				if isConnRefused(err) {
					<-time.After(1 * time.Millisecond)
					continue
				}
				if err != nil {
					log.Debug("Reconnecting after an error")
					<-time.After(1 * time.Millisecond)
					continue
				}

				synC <- &dialResult{c, err}
				return
			}
		}
	}()
	select {
	case dr := <-synC:
		return dr.c, dr.err
	case <-time.After(timeout):
		close(stopC)
		go func() {
			dr := <-synC
			if dr != nil && dr.c != nil {
				dr.c.Close()
			}
		}()
		return nil, errors.Errorf("dial %s: timeout", address)
	}
}
