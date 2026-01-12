// MIT License
//
// Copyright (c) 2023 Georgiy Lebedev, Plamen Petrov and vHive team
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

package ctriface

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/vhive-serverless/vhive/devmapper"
	"github.com/vhive-serverless/vhive/snapshotting"
	"github.com/vhive-serverless/vhive/storage"

	log "github.com/sirupsen/logrus"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/namespaces"

	fcclient "github.com/firecracker-microvm/firecracker-containerd/firecracker-control/client"
	"github.com/firecracker-microvm/firecracker-containerd/proto"

	// note: from the original repo

	_ "google.golang.org/grpc/codes"  //tmp
	_ "google.golang.org/grpc/status" //tmp

	"github.com/vhive-serverless/vhive/ctriface/image"
	"github.com/vhive-serverless/vhive/memory/manager"
	"github.com/vhive-serverless/vhive/metrics"
	"github.com/vhive-serverless/vhive/misc"

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

// RegistryCredentials represents the credentials for a single Docker registry.
type RegistryCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// DockerCredentials wraps the credential mapping used for container image registries.
type DockerCredentials struct {
	DockerCredentials map[string]RegistryCredentials `json:"docker-credentials"`
}

// ShimPool manages a pool of pre-created shims for faster VM startup
type ShimPool struct {
	mu            sync.Mutex
	availableVMID []string         // Pool of available pre-created shim VM IDs
	fcClient      *fcclient.Client // Firecracker client for creating shims
	poolSize      int              // Target pool size
	logger        *log.Entry
	counter       int // Counter for generating unique VM IDs
}

// NewShimPool creates a new shim pool
func NewShimPool(fcClient *fcclient.Client, poolSize int) *ShimPool {
	return &ShimPool{
		availableVMID: make([]string, 0, poolSize),
		fcClient:      fcClient,
		poolSize:      poolSize,
		logger:        log.WithField("component", "ShimPool"),
		counter:       0,
	}
}

// generateVMID creates a new unique VM ID
func (sp *ShimPool) generateVMID() string {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.counter++
	return fmt.Sprintf("shim-%d-%d", os.Getpid(), sp.counter)
}

// AcquireShim gets a pre-created shim from the pool, or creates a new one if pool is empty
func (sp *ShimPool) AcquireShim(ctx context.Context) (string, error) {
	sp.mu.Lock()
	// Try to get a pre-created shim from the pool
	if len(sp.availableVMID) > 0 {
		vmID := sp.availableVMID[0]
		sp.availableVMID = sp.availableVMID[1:]
		sp.mu.Unlock()
		sp.logger.WithField("vmID", vmID).Debug("Acquired pre-created shim from pool")

		// Asynchronously refill the pool
		go sp.refillPool(ctx)

		return vmID, nil
	}
	sp.mu.Unlock()

	// Pool is empty, create a new shim on-demand
	vmID := sp.generateVMID()
	sp.logger.WithField("vmID", vmID).Debug("Pool empty, creating new shim on-demand")

	if err := sp.createShim(ctx, vmID); err != nil {
		return "", err
	}

	// Asynchronously refill the pool
	go sp.refillPool(ctx)

	return vmID, nil
}

// ReleaseShim removes a shim and removes it from tracking
func (sp *ShimPool) ReleaseShim(ctx context.Context, vmID string) error {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	// Always remove the shim (no reuse)
	sp.logger.WithField("vmID", vmID).Debug("Removing shim")
	return sp.removeShim(ctx, vmID)
}

// createShim creates a new shim using the PrepareShim API
func (sp *ShimPool) createShim(ctx context.Context, vmID string) error {
	sp.logger.WithField("vmID", vmID).Debug("Creating new shim")

	ctx = namespaces.WithNamespace(ctx, vmID)
	_, err := sp.fcClient.PrepareShim(ctx, &proto.PrepareShimRequest{
		VMID: vmID,
	})
	if err != nil {
		sp.logger.WithField("vmID", vmID).WithError(err).Error("Failed to prepare shim")
		return err
	}

	sp.logger.WithField("vmID", vmID).Debug("Successfully created shim")
	return nil
}

// removeShim removes a shim using the RemoveShim API
func (sp *ShimPool) removeShim(ctx context.Context, vmID string) error {
	sp.logger.WithField("vmID", vmID).Debug("Removing shim")

	ctx = namespaces.WithNamespace(ctx, vmID)
	_, err := sp.fcClient.RemoveShim(ctx, &proto.RemoveShimRequest{
		VMID: vmID,
	})
	if err != nil {
		sp.logger.WithField("vmID", vmID).WithError(err).Error("Failed to remove shim")
		return err
	}

	sp.logger.WithField("vmID", vmID).Debug("Successfully removed shim")
	return nil
}

// refillPool ensures the pool has the target number of pre-created shims
func (sp *ShimPool) refillPool(ctx context.Context) {
	sp.mu.Lock()
	currentSize := len(sp.availableVMID)
	needed := sp.poolSize - currentSize
	sp.mu.Unlock()

	if needed <= 0 {
		return
	}

	sp.logger.WithField("needed", needed).Debug("Refilling shim pool")

	for i := 0; i < needed; i++ {
		vmID := sp.generateVMID()
		if err := sp.createShim(ctx, vmID); err != nil {
			sp.logger.WithField("vmID", vmID).WithError(err).Error("Failed to create shim during pool refill")
			continue
		}

		sp.mu.Lock()
		sp.availableVMID = append(sp.availableVMID, vmID)
		sp.mu.Unlock()
	}

	sp.logger.WithField("poolSize", len(sp.availableVMID)).Debug("Pool refilled")
}

// InitializePool pre-creates shims to fill the pool
func (sp *ShimPool) InitializePool(ctx context.Context) error {
	sp.logger.WithField("poolSize", sp.poolSize).Info("Initializing shim pool")

	for i := 0; i < sp.poolSize; i++ {
		vmID := sp.generateVMID()
		if err := sp.createShim(ctx, vmID); err != nil {
			sp.logger.WithError(err).Error("Failed to initialize shim pool")
			return err
		}

		sp.mu.Lock()
		sp.availableVMID = append(sp.availableVMID, vmID)
		sp.mu.Unlock()
	}

	sp.logger.WithField("poolSize", len(sp.availableVMID)).Info("Shim pool initialized")
	return nil
}

// Cleanup removes all shims from the pool
func (sp *ShimPool) Cleanup(ctx context.Context) error {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	sp.logger.Info("Cleaning up shim pool")

	var errors []error

	// Remove all available shims
	for _, vmID := range sp.availableVMID {
		if err := sp.removeShim(ctx, vmID); err != nil {
			errors = append(errors, err)
		}
	}

	sp.availableVMID = nil

	if len(errors) > 0 {
		sp.logger.WithField("errorCount", len(errors)).Error("Errors during shim pool cleanup")
		return errors[0] // Return the first error
	}

	sp.logger.Debug("Shim pool cleaned up")
	return nil
}

// GetPoolStats returns statistics about the shim pool
func (sp *ShimPool) GetPoolStats() (available int) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	return len(sp.availableVMID)
}

// Orchestrator Drives all VMs
type Orchestrator struct {
	vmPool            *misc.VMPool
	shimPool          *ShimPool
	cachedImages      map[string]containerd.Image
	workloadIo        sync.Map // vmID string -> WorkloadIoWriter
	snapshotter       string
	client            *containerd.Client
	fcClient          *fcclient.Client
	devMapper         *devmapper.DeviceMapper
	imageManager      *image.ImageManager
	snapshotManager   *snapshotting.SnapshotManager
	dockerCredentials DockerCredentials
	// store *skv.KVStore
	snapshotMode      string
	cacheSnaps        bool
	isUPFEnabled      bool
	isLazyMode        bool
	isWSPulling       bool
	isChunkingEnabled bool
	chunkSize         uint64
	snapshotsDir      string
	snapshotsStorage  string
	snapshotsBucket   string
	baseSnap          bool
	isMetricsMode     bool
	netPoolSize       int
	shimPoolSize      int
	cacheSize         uint64
	threads           int
	encryption        bool

	vethPrefix  string
	clonePrefix string

	minioAddr      string
	minioAccessKey string
	minioSecretKey string

	memoryManager *manager.MemoryManager

	securityMode string
}

// NewOrchestrator Initializes a new orchestrator
func NewOrchestrator(snapshotter, hostIface string, opts ...OrchestratorOption) *Orchestrator {
	var err error

	o := new(Orchestrator)
	o.cachedImages = make(map[string]containerd.Image)
	o.snapshotter = snapshotter
	o.snapshotsDir = "/fccd/snapshots"
	o.snapshotsBucket = "snapshots"
	o.netPoolSize = 10
	o.shimPoolSize = 5 // Default shim pool size
	o.vethPrefix = "172.17"
	o.clonePrefix = "172.18"
	o.minioAddr = "10.96.0.46:9000"
	o.minioAccessKey = "minio"
	o.minioSecretKey = "minio123"

	for _, opt := range opts {
		opt(o)
	}

	o.vmPool = misc.NewVMPool(hostIface, o.netPoolSize, o.vethPrefix, o.clonePrefix)

	if _, err := os.Stat(o.snapshotsDir); err != nil {
		if !os.IsNotExist(err) {
			log.Panicf("Snapshot dir %s exists", o.snapshotsDir)
		}
	}

	if err := os.MkdirAll(o.snapshotsDir, 0777); err != nil {
		log.Panicf("Failed to create snapshots dir %s", o.snapshotsDir)
	}

	if o.GetUPFEnabled() {
		managerCfg := manager.MemoryManagerCfg{
			MetricsModeOn: o.isMetricsMode,
		}
		o.memoryManager = manager.NewMemoryManager(managerCfg)
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

	// Initialize shim pool
	if o.shimPoolSize > 0 {
		log.WithField("poolSize", o.shimPoolSize).Info("Initializing shim pool")
		o.shimPool = NewShimPool(o.fcClient, o.shimPoolSize)
		o.shimPool.InitializePool(context.Background())
	}

	o.devMapper = devmapper.NewDeviceMapper(o.client)
	o.imageManager = image.NewImageManager(o.client, o.snapshotter)

	snapshotsBucket := o.GetSnapshotsBucket()

	var objectStore storage.ObjectStorage
	if o.GetSnapshotMode() == "remote" {
		minioClient, _ := minio.New(o.GetMinioAddr(), &minio.Options{
			Creds:  credentials.NewStaticV4(o.GetMinioAccessKey(), o.GetMinioSecretKey(), ""),
			Secure: false,
		})

		var err error
		objectStore, err = storage.NewMinioStorage(minioClient, snapshotsBucket)
		if err != nil {
			log.WithError(err).Fatalf("failed to create MinIO storage for snapshots in bucket %s", snapshotsBucket)
		}
	}
	o.snapshotManager = snapshotting.NewSnapshotManager(o.snapshotsStorage, objectStore, o.isChunkingEnabled, false,
		o.isLazyMode, o.isWSPulling, o.chunkSize, o.cacheSize, o.securityMode, o.threads, o.encryption)

	return o
}

func (o *Orchestrator) setupCloseHandler() {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Info("\r- Ctrl+C pressed in Terminal")
		// _ = o.StopActiveVMs()
		o.Cleanup()
		os.Exit(0)
	}()
}

// Cleanup Removes the bridges created by the VM pool's tap manager
// Cleans up snapshots directory and shim pool
func (o *Orchestrator) Cleanup() {
	o.vmPool.CleanupNetwork()

	// Cleanup shim pool
	if o.shimPool != nil {
		ctx := context.Background()
		if err := o.shimPool.Cleanup(ctx); err != nil {
			log.WithError(err).Error("Failed to cleanup shim pool")
		}
	}

	if err := os.RemoveAll(o.snapshotsDir); err != nil {
		log.Panic("failed to delete snapshots dir", err)
	}

	o.snapshotManager.WriteHitStatsToCSV(o.snapshotsStorage + "/hit_rates.csv")
	o.snapshotManager.WriteAccessHistoryToTextFile(o.snapshotsStorage + "/access.txt")

	o.StopActiveVMs()
}

// GetSnapshotMode Returns the snapshots mode of the orchestrator
func (o *Orchestrator) GetSnapshotMode() string {
	return o.snapshotMode
}

func (o *Orchestrator) GetCacheSnaps() bool {
	return o.cacheSnaps
}

// GetUPFEnabled Returns the UPF mode of the orchestrator
func (o *Orchestrator) GetUPFEnabled() bool {
	return o.isUPFEnabled
}

// DumpUPFPageStats Dumps the memory manager's stats about the number of
// the unique pages and the number of the pages that are reused across invocations
func (o *Orchestrator) DumpUPFPageStats(vmID, functionName, metricsOutFilePath string) error {
	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Orchestrator received DumpUPFPageStats")

	return o.memoryManager.DumpUPFPageStats(vmID, functionName, metricsOutFilePath)
}

// DumpUPFLatencyStats Dumps the memory manager's latency stats
func (o *Orchestrator) DumpUPFLatencyStats(vmID, functionName, latencyOutFilePath string) error {
	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Orchestrator received DumpUPFPageStats")

	return o.memoryManager.DumpUPFLatencyStats(vmID, functionName, latencyOutFilePath)
}

// GetUPFLatencyStats Returns the memory manager's latency stats
func (o *Orchestrator) GetUPFLatencyStats(vmID string) ([]*metrics.Metric, error) {
	logger := log.WithFields(log.Fields{"vmID": vmID})
	logger.Debug("Orchestrator received DumpUPFPageStats")

	return o.memoryManager.GetUPFLatencyStats(vmID)
}

// GetSnapshotsDir Returns the orchestrator's snapshot directory
func (o *Orchestrator) GetSnapshotsDir() string {
	return o.snapshotsDir
}

func (o *Orchestrator) getSnapshotFile(vmID string) string {
	return filepath.Join(o.getVMBaseDir(vmID), "snap_file")
}

func (o *Orchestrator) getMemoryFile(vmID string) string {
	return filepath.Join(o.getVMBaseDir(vmID), "mem_file")
}

func (o *Orchestrator) getWorkingSetFile(vmID string) string {
	return filepath.Join(o.getVMBaseDir(vmID), "working_set_pages")
}

func (o *Orchestrator) getVMBaseDir(vmID string) string {
	return filepath.Join(o.snapshotsDir, vmID)
}

func (o *Orchestrator) GetDockerCredentials() string {
	data, err := json.Marshal(o.dockerCredentials)
	if err != nil {
		// Safe fallback: empty JSON
		return "{}"
	}
	return string(data)
}

// GetSnapshotsBucket returns the S3 bucket name used by the orchestrator for storing remote snapshots.
func (o *Orchestrator) GetSnapshotsBucket() string {
	return o.snapshotsBucket
}

// GetMinioAddr returns the address (endpoint) of the MinIO server used by the orchestrator.
func (o *Orchestrator) GetMinioAddr() string {
	return o.minioAddr
}

// GetMinioAccessKey returns the access key used to authenticate with the MinIO server.
func (o *Orchestrator) GetMinioAccessKey() string {
	return o.minioAccessKey
}

// GetMinioSecretKey returns the secret key used to authenticate with the MinIO server.
// This should be handled securely and never exposed in logs or error messages.
func (o *Orchestrator) GetMinioSecretKey() string {
	return o.minioSecretKey
}

func (o *Orchestrator) GetSnapshotManager() *snapshotting.SnapshotManager {
	return o.snapshotManager
}

// InitializeShimPool initializes the shim pool by pre-creating shims
func (o *Orchestrator) InitializeShimPool(ctx context.Context) error {
	if o.shimPool == nil {
		return fmt.Errorf("shim pool is not enabled")
	}
	return o.shimPool.InitializePool(ctx)
}

// AcquireShimFromPool gets a pre-created shim from the pool
// Returns a VM ID that should be used for creating the VM
func (o *Orchestrator) AcquireShimFromPool(ctx context.Context) (string, error) {
	if o.shimPool == nil {
		return "", fmt.Errorf("shim pool is not enabled")
	}
	return o.shimPool.AcquireShim(ctx)
}

// ReleaseShimToPool releases a shim and removes it (shims are never reused)
func (o *Orchestrator) ReleaseShimToPool(ctx context.Context, vmID string) error {
	if o.shimPool == nil {
		return nil // Silently ignore if pool is not enabled
	}
	return o.shimPool.ReleaseShim(ctx, vmID)
}

// GetShimPoolStats returns statistics about the shim pool
func (o *Orchestrator) GetShimPoolStats() (available int) {
	if o.shimPool == nil {
		return 0
	}
	return o.shimPool.GetPoolStats()
}

func (o *Orchestrator) setupHeartbeat() {
	heartbeat := time.NewTicker(60 * time.Second)

	go func() {
		for {
			<-heartbeat.C
			log.Info("HEARTBEAT: number of active VMs: ", len(o.vmPool.GetVMMap()))
		} // for
	}() // go func
}
