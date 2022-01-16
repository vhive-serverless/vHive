// MIT License
//
// Copyright (c) 2020 Plamen Petrov and EASE lab
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

package deduplicated

import (
	"github.com/ease-lab/vhive/ctrimages"
	"github.com/ease-lab/vhive/devmapper"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/containerd/containerd"

	fcclient "github.com/firecracker-microvm/firecracker-containerd/firecracker-control/client"
	// note: from the original repo

	_ "google.golang.org/grpc/codes"  //tmp
	_ "google.golang.org/grpc/status" //tmp

	"github.com/ease-lab/vhive/metrics"
	"github.com/ease-lab/vhive/misc"

	_ "github.com/davecgh/go-spew/spew" //tmp
)

const (
	containerdAddress      = "/run/firecracker-containerd/containerd.sock"
	containerdTTRPCAddress = containerdAddress + ".ttrpc"
	namespaceName          = "firecracker-containerd"
)

type WorkloadIoWriter struct {
	logger *log.Entry
}

func NewWorkloadIoWriter(vmID string) WorkloadIoWriter {
	return WorkloadIoWriter{log.WithFields(log.Fields{"vmID": vmID})}
}

func (wio WorkloadIoWriter) Write(p []byte) (n int, err error) {
	s := string(p)
	lines := strings.Split(s, "\n")
	for i := range lines {
		wio.logger.Info(string(lines[i]))
	}
	return len(p), nil
}

// DedupOrchestrator Drives all VMs
type DedupOrchestrator struct {
	vmPool       *misc.VMPool
	workloadIo   sync.Map // vmID string -> WorkloadIoWriter
	snapshotter  string
	client       *containerd.Client
	fcClient     *fcclient.Client
	devMapper    *devmapper.DeviceMapper
	imageManager *ctrimages.ImageManager
	// store *skv.KVStore
	snapshotsEnabled bool
	isUPFEnabled     bool
	isLazyMode       bool
	snapshotsDir     string
	isMetricsMode    bool
	hostIface        string
}

// NewDedupOrchestrator Initializes a new orchestrator
func NewDedupOrchestrator(snapshotter, hostIface, poolName, metadataDev string, netPoolSize int, opts ...OrchestratorOption) *DedupOrchestrator { // TODO: args
	var err error

	o := new(DedupOrchestrator)
	o.vmPool = misc.NewVMPool(hostIface, netPoolSize)
	o.snapshotter = snapshotter
	o.snapshotsDir = "/fccd/snapshots"
	o.hostIface = hostIface

	for _, opt := range opts {
		opt(o)
	}

	if _, err := os.Stat(o.snapshotsDir); err != nil {
		if !os.IsNotExist(err) {
			log.Panicf("Snapshot dir %s exists", o.snapshotsDir)
		}
	}

	if err := os.MkdirAll(o.snapshotsDir, 0777); err != nil {
		log.Panicf("Failed to create snapshots dir %s", o.snapshotsDir)
	}

	log.Info("Creating containerd client")
	o.client, err = containerd.New(containerdAddress)
	if err != nil {
		log.Fatal("Failed to start containerd client", err)
	}
	log.Info("Created containerd client")

	log.Info("Creating firecracker client")
	o.fcClient, err = fcclient.New(containerdTTRPCAddress)
	if err != nil {
		log.Fatal("Failed to start firecracker client", err)
	}
	log.Info("Created firecracker client")

	o.devMapper = devmapper.NewDeviceMapper(o.client, poolName, metadataDev)

	o.imageManager = ctrimages.NewImageManager(o.client, o.snapshotter)

	return o
}

func (o *DedupOrchestrator) setupCloseHandler() {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Info("\r- Ctrl+C pressed in Terminal")
		_ = o.StopActiveVMs()
		o.Cleanup()
		os.Exit(0)
	}()
}

// Cleanup Removes the bridges created by the VM pool's tap manager
// Cleans up snapshots directory
func (o *DedupOrchestrator) Cleanup() {
	o.vmPool.CleanupNetwork()
	if err := os.RemoveAll(o.snapshotsDir); err != nil {
		log.Panic("failed to delete snapshots dir", err)
	}
}

// GetSnapshotsEnabled Returns the snapshots mode of the orchestrator
func (o *DedupOrchestrator) GetSnapshotsEnabled() bool {
	return o.snapshotsEnabled
}

// GetUPFEnabled Returns the UPF mode of the orchestrator
func (o *DedupOrchestrator) GetUPFEnabled() bool {
	return false
}

// DumpUPFPageStats Dumps the memory manager's stats about the number of
// the unique pages and the number of the pages that are reused across invocations
func (o *DedupOrchestrator) DumpUPFPageStats(vmID, functionName, metricsOutFilePath string) error {
	return nil
}

// DumpUPFLatencyStats Dumps the memory manager's latency stats
func (o *DedupOrchestrator) DumpUPFLatencyStats(vmID, functionName, latencyOutFilePath string) error {
	return nil
}

// GetUPFLatencyStats Returns the memory manager's latency stats
func (o *DedupOrchestrator) GetUPFLatencyStats(vmID string) ([]*metrics.Metric, error) {
	return make([]*metrics.Metric, 0), nil
}

func (o *DedupOrchestrator) getVMBaseDir(vmID string) string {
	return filepath.Join(o.snapshotsDir, vmID)
}

func (o *DedupOrchestrator) setupHeartbeat() {
	heartbeat := time.NewTicker(60 * time.Second)

	go func() {
		for {
			<-heartbeat.C
			log.Info("HEARTBEAT: number of active VMs: ", len(o.vmPool.GetVMMap()))
		} // for
	}() // go func
}
