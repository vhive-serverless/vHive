// MIT License
//
// Copyright (c) 2023 Georgiy Lebedev, Amory Hoste and vHive team
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

package snapshotting

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/vhive-serverless/vhive/storage"

	"github.com/vhive-serverless/vhive/k8s"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	ctrlLog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func init() {
	ctrlLog.SetLogger(zap.New(zap.UseDevMode(true)))
}

// SnapshotManager manages snapshots stored on the node.
type SnapshotManager struct {
	sync.Mutex
	// Stored snapshots (identified by the function instance revision, which is provided by the `K_REVISION` environment
	// variable of knative).
	snapshots           map[string]*Snapshot
	baseFolder          string
	cacheCapacityBytes  int64  // Maximum cache capacity in bytes
	// Used to store remote snapshots
	storage             storage.ObjectStorage
}

func NewSnapshotManager(baseFolder string, store storage.ObjectStorage, skipCleanup bool) *SnapshotManager {
	return NewSnapshotManagerWithCapacity(baseFolder, store, skipCleanup, 10*1024*1024*1024) // 10GB default
}

func NewSnapshotManagerWithCapacity(baseFolder string, store storage.ObjectStorage, skipCleanup bool, capacityBytes int64) *SnapshotManager {
	manager := &SnapshotManager{
		snapshots:          make(map[string]*Snapshot),
		baseFolder:         baseFolder,
		cacheCapacityBytes: capacityBytes,
		storage:            store,
	}

	// Clean & init basefolder unless skipping is requested
	if !skipCleanup {
		_ = os.RemoveAll(manager.baseFolder)
	}
	_ = os.MkdirAll(manager.baseFolder, os.ModePerm)

	return manager
}

// AcquireSnapshot returns a snapshot for the specified revision if it is available.
func (mgr *SnapshotManager) AcquireSnapshot(revision string) (*Snapshot, error) {
	mgr.Lock()
	defer mgr.Unlock()

	// Check if idle snapshot is available for the given image
	snap, ok := mgr.snapshots[revision]
	if !ok {
		return nil, errors.New(fmt.Sprintf("Get: Snapshot for revision %s does not exist", revision))
	}

	// Snapshot registered in manager but creation not finished yet
	if !snap.ready {
		return nil, errors.New("Snapshot is not yet usable")
	}

	// Update access timestamp for LRU tracking
	snap.UpdateLastAccessedTimestamp()

	// Return snapshot for supplied revision
	return mgr.snapshots[revision], nil
}

// InitSnapshot initializes a snapshot by adding its metadata to the SnapshotManager. Once the snapshot has
// been created, CommitSnapshot must be run to finalize the snapshot creation and make the snapshot available for use.
func (mgr *SnapshotManager) InitSnapshot(revision, image string) (*Snapshot, error) {
	// Use default memory size estimation of 1GB if not specified
	return mgr.InitSnapshotWithEviction(revision, image, 1024*1024*1024)
}

// CommitSnapshot finalizes the snapshot creation and makes it available for use.
func (mgr *SnapshotManager) CommitSnapshot(revision string) error {
	mgr.Lock()
	defer mgr.Unlock()

	snap, ok := mgr.snapshots[revision]
	if !ok {
		return errors.New(fmt.Sprintf("Snapshot for revision %s to commit does not exist", revision))
	}

	if snap.ready {
		return errors.New(fmt.Sprintf("Snapshot for revision %s has already been committed", revision))
	}

	// Calculate and set snapshot size
	if err := snap.CalculateAndSetSize(); err != nil {
		return errors.Wrapf(err, "failed to calculate snapshot size for revision %s", revision)
	}

	snap.ready = true

	if err := mgr.updateNodeSnapshotCRD(revision); err != nil {
		return errors.Wrapf(err, "updating NodeSnapshotCache CRD for revision %s", revision)
	}

	return nil
}

// DeleteSnapshot removes the snapshot for the specified revision from the manager
func (mgr *SnapshotManager) DeleteSnapshot(revision string) error {
	mgr.Lock()
	defer mgr.Unlock()

	snap, ok := mgr.snapshots[revision]
	if !ok {
		return errors.New(fmt.Sprintf("Delete: Snapshot for revision %s does not exist", revision))
	}

	_ = snap.Cleanup()

	delete(mgr.snapshots, revision)

	return nil
}

// UploadSnapshot Uploads a snapshot to MinIO.
// A manifest is created and uploaded to MinIO to describe the snapshot contents.
func (mgr *SnapshotManager) UploadSnapshot(revision string) error {
	snap, err := mgr.AcquireSnapshot(revision)
	if err != nil {
		return errors.Wrapf(err, "acquiring snapshot")
	}

	files := []string{
		snap.GetMemFilePath(),
		snap.GetSnapshotFilePath(),
		snap.GetInfoFilePath(),
	}

	for _, filePath := range files {
		if err := mgr.uploadFile(revision, filePath); err != nil {
			return err
		}
	}

	return nil
}

// DownloadSnapshot downloads a snapshot from MinIO.
func (mgr *SnapshotManager) DownloadSnapshot(revision string) (*Snapshot, error) {
	snap, err := mgr.InitSnapshot(revision, "")
	if err != nil {
		return nil, errors.Wrapf(err, "initializing snapshot for revision %s", revision)
	}

	defer func() {
		// Clean up if the snapshot wasn't committed
		if !snap.ready {
			_ = mgr.DeleteSnapshot(revision)
		}
	}()

	// Download and save the info file (manifest)
	infoPath := snap.GetInfoFilePath()
	infoName := filepath.Base(infoPath)
	if err := mgr.downloadFile(revision, infoPath, infoName); err != nil {
		return nil, errors.Wrapf(err, "downloading manifest for snapshot %s", revision)
	}

	if err := snap.LoadSnapInfo(infoPath); err != nil {
		return nil, errors.Wrapf(err, "loading manifest from %s", infoPath)
	}

	// Download remaining snapshot files
	files := []string{
		snap.GetSnapshotFilePath(),
		snap.GetMemFilePath(),
	}
	for _, filePath := range files {
		fileName := filepath.Base(filePath)
		if err := mgr.downloadFile(revision, filePath, fileName); err != nil {
			return nil, errors.Wrapf(err, "downloading file %s", fileName)
		}
	}

	if err := mgr.CommitSnapshot(revision); err != nil {
		return nil, errors.Wrap(err, "committing snapshot")
	}

	return snap, nil
}

// uploadFile uploads a single file to MinIO under the specified revision and file name.
func (mgr *SnapshotManager) uploadFile(revision, filePath string) error {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return errors.Wrapf(err, "getting file info for %s", filePath)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return errors.Wrapf(err, "opening file %s", filePath)
	}
	defer file.Close()

	objectKey := mgr.getObjectKey(revision, filepath.Base(filePath))
	return mgr.storage.UploadObject(objectKey, file, fileInfo.Size())
}

// downloadFile Downloads a file from MinIO and save it to the specified path
func (mgr *SnapshotManager) downloadFile(revision, filePath, fileName string) error {
	objectKey := mgr.getObjectKey(revision, fileName)
	obj, err := mgr.storage.DownloadObject(objectKey)
	if err != nil {
		return err
	}
	defer obj.Close()

	outFile, err := os.Create(filePath)
	if err != nil {
		return errors.Wrap(err, "creating output file")
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, obj); err != nil {
		return errors.Wrap(err, "writing file")
	}
	return nil
}

// SnapshotExistsComplete checks if all required snapshot files exist in remote storage
func (mgr *SnapshotManager) SnapshotExists(revision string) (bool, error) {
	// Create a temporary snapshot to get the expected file names
	snap, err := mgr.InitSnapshot(revision, "")
	if err != nil {
		return false, errors.Wrapf(err, "initializing snapshot for existence check")
	}

	defer func() {
		// Clean up the temporary snapshot
		_ = mgr.DeleteSnapshot(revision)
	}()

	requiredFiles := []string{
		filepath.Base(snap.GetMemFilePath()),
		filepath.Base(snap.GetSnapshotFilePath()),
		filepath.Base(snap.GetInfoFilePath()),
	}

	// Check each file exists
	for _, fileName := range requiredFiles {
		objectKey := mgr.getObjectKey(revision, fileName)
		exists, err := mgr.storage.Exists(objectKey)
		if err != nil {
			return false, errors.Wrapf(err, "checking if file %s exists for snapshot %s", fileName, revision)
		}
		if !exists {
			return false, nil // At least one required file is missing
		}
	}

	return true, nil
}

// Helper function to construct object keys (you may need to adjust this based on your key structure)
func (mgr *SnapshotManager) getObjectKey(revision, fileName string) string {
	return fmt.Sprintf("%s/%s", revision, fileName)
}

// TODO probably move this to a separate package
func newK8sClient() (client.Client, error) {
	scheme := runtime.NewScheme()
	if err := k8s.AddToScheme(scheme); err != nil {
		return nil, err
	}

	// Try in-cluster config first
	cfg, err := config.GetConfig()
	if err != nil {
		// Fallback to kubeconfig file
		cfg, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
		if err != nil {
			return nil, err
		}
	}

	return client.New(cfg, client.Options{Scheme: scheme})
}

// updateNodeSnapshotCRD updates the NodeSnapshotCache CRD with the given revision.
func (mgr *SnapshotManager) updateNodeSnapshotCRD(revision string) error {
	nodeName, _ := os.Hostname()
	k8sClient, err := newK8sClient()
	if err != nil {
		return err
	}

	ctx := context.TODO()
	crName := nodeName // use the node's hostname as the CR name

	cr := &k8s.NodeSnapshotCache{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: crName}, cr)
	if err != nil && apierrors.IsNotFound(err) {
		// create it
		cr = &k8s.NodeSnapshotCache{
			ObjectMeta: metav1.ObjectMeta{
				Name:      crName,
				Namespace: "default",
			},
			Spec: k8s.NodeSnapshotCacheSpec{
				NodeName:  nodeName,
				Snapshots: []string{revision},
			},
		}
		return k8sClient.Create(ctx, cr)
	} else if err != nil {
		return err
	}

	// already exists, update it
	for _, existing := range cr.Spec.Snapshots {
		if existing == revision {
			return nil // already listed
		}
	}
	cr.Spec.Snapshots = append(cr.Spec.Snapshots, revision)
	
	return k8sClient.Update(ctx, cr)
}

// getCurrentCacheSize calculates the total size of all snapshots in the cache
func (mgr *SnapshotManager) getCurrentCacheSize() int64 {
	var totalSize int64
	for _, snap := range mgr.snapshots {
		if snap.ready {
			totalSize += snap.GetSizeInBytes()
		}
	}
	return totalSize
}

// evictLRUSnapshots evicts least recently used snapshots until there is enough free space
func (mgr *SnapshotManager) evictLRUSnapshots(requiredSpace int64) error {
	for mgr.getCurrentCacheSize()+requiredSpace > mgr.cacheCapacityBytes {
		lruSnapshot := mgr.findLRUSnapshot()
		if lruSnapshot == nil {
			// No more snapshots to evict, but still not enough space
			return errors.New("insufficient space: cannot evict more snapshots")
		}

		log.WithFields(log.Fields{
			"revision":   lruSnapshot.GetId(),
			"size":       lruSnapshot.GetSizeInBytes(),
			"lastAccess": lruSnapshot.GetLastAccessedTimestamp(),
		}).Info("Evicting LRU snapshot")

		if err := mgr.evictSnapshot(lruSnapshot.GetId()); err != nil {
			return errors.Wrapf(err, "failed to evict snapshot %s", lruSnapshot.GetId())
		}
	}
	return nil
}

// findLRUSnapshot finds the least recently used ready snapshot
func (mgr *SnapshotManager) findLRUSnapshot() *Snapshot {
	var lruSnapshot *Snapshot
	
	for _, snap := range mgr.snapshots {
		if !snap.ready {
			continue // Skip non-ready snapshots
		}
		
		if lruSnapshot == nil || snap.GetLastAccessedTimestamp().Before(lruSnapshot.GetLastAccessedTimestamp()) {
			lruSnapshot = snap
		}
	}
	
	return lruSnapshot
}

// evictSnapshot removes a snapshot from both disk and CRD
func (mgr *SnapshotManager) evictSnapshot(revision string) error {
	// Remove from CRD first
	if err := mgr.removeFromNodeSnapshotCRD(revision); err != nil {
		log.WithError(err).WithField("revision", revision).Warn("Failed to remove snapshot from CRD during eviction")
		// Continue with disk cleanup even if CRD update fails
	}

	// Remove from disk and memory
	return mgr.DeleteSnapshot(revision)
}

// removeFromNodeSnapshotCRD removes a snapshot from the NodeSnapshotCache CRD
func (mgr *SnapshotManager) removeFromNodeSnapshotCRD(revision string) error {
	nodeName, _ := os.Hostname()
	k8sClient, err := newK8sClient()
	if err != nil {
		return err
	}

	ctx := context.TODO()
	crName := nodeName

	cr := &k8s.NodeSnapshotCache{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: crName}, cr)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil // CRD doesn't exist, nothing to remove
		}
		return err
	}

	// Find and remove the snapshot from the list
	var updatedSnapshots []string
	for _, snap := range cr.Spec.Snapshots {
		if snap != revision {
			updatedSnapshots = append(updatedSnapshots, snap)
		}
	}

	cr.Spec.Snapshots = updatedSnapshots
	return k8sClient.Update(ctx, cr)
}

// InitSnapshotWithEviction initializes a snapshot with LRU eviction
func (mgr *SnapshotManager) InitSnapshotWithEviction(revision, image string, estimatedMemorySize int64) (*Snapshot, error) {
	mgr.Lock()
	defer mgr.Unlock()

	logger := log.WithFields(log.Fields{"revision": revision, "image": image})
	logger.Debug("Initializing snapshot with eviction policy")

	if _, present := mgr.snapshots[revision]; present {
		return nil, errors.New(fmt.Sprintf("Add: Snapshot for revision %s already exists", revision))
	}

	// Create snapshot object temporarily to estimate size
	tempSnap := NewSnapshot(revision, mgr.baseFolder, image)
	estimatedSize := tempSnap.EstimateSnapshotSize(estimatedMemorySize)

	logger.WithFields(log.Fields{
		"estimatedSize":       estimatedSize,
		"currentCacheSize":    mgr.getCurrentCacheSize(),
		"cacheCapacity":       mgr.cacheCapacityBytes,
	}).Debug("Checking cache capacity before snapshot creation")

	// Check if snapshot size exceeds total capacity
	if estimatedSize > mgr.cacheCapacityBytes {
		return nil, errors.New(fmt.Sprintf("Snapshot estimated size (%d bytes) exceeds cache capacity (%d bytes)", estimatedSize, mgr.cacheCapacityBytes))
	}

	// Evict LRU snapshots if necessary
	if err := mgr.evictLRUSnapshots(estimatedSize); err != nil {
		return nil, errors.Wrapf(err, "failed to evict snapshots for revision %s", revision)
	}

	// Create snapshot object and move into creating state
	snap := NewSnapshot(revision, mgr.baseFolder, image)
	mgr.snapshots[snap.GetId()] = snap

	// Create directory to store snapshot data
	err := snap.CreateSnapDir()
	if err != nil {
		delete(mgr.snapshots, snap.GetId()) // Clean up on failure
		return nil, errors.Wrapf(err, "creating snapDir for snapshots %s", revision)
	}

	return snap, nil
}

// GetCacheCapacity returns the cache capacity in bytes
func (mgr *SnapshotManager) GetCacheCapacity() int64 {
	return mgr.cacheCapacityBytes
}

// SetCacheCapacity sets the cache capacity in bytes
func (mgr *SnapshotManager) SetCacheCapacity(capacityBytes int64) {
	mgr.Lock()
	defer mgr.Unlock()
	mgr.cacheCapacityBytes = capacityBytes
}

// GetCacheUsage returns current cache usage statistics
func (mgr *SnapshotManager) GetCacheUsage() (int64, int64, int) {
	mgr.Lock()
	defer mgr.Unlock()
	
	currentSize := mgr.getCurrentCacheSize()
	return currentSize, mgr.cacheCapacityBytes, len(mgr.snapshots)
}
