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
	"archive/tar"
	"container/heap"
	"container/list"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	// "math/rand"

	"github.com/pkg/errors"

	log "github.com/sirupsen/logrus"

	"github.com/vhive-serverless/vhive/storage"
)

const (
	chunkPrefix     = "_chunks"
	K               = 3   // TODO: tune
	deleteBatchSize = 150 // TODO: tune
)

var imageChunks = map[string]map[[16]byte]bool{}
var rootfsChunks = map[[16]byte]bool{}
var baseSnapChunks = map[[16]byte]bool{}
var EncryptionKey = []byte("vhive-snapshot-enc") // 16 bytes key for AES-128

func (mgr *SnapshotManager) GetChunkSize() uint64 {
	return mgr.chunkSize
}

type ChunkEntry struct {
	hash           string
	accessTimes    []time.Time
	element        *list.Element // used when in coldList
	containingList *list.List    // nil when in hotHeap
	heapIndex      int           // index in hotHeap, -1 when in coldList
}

// HotHeap implements heap.Interface for ChunkEntry items
// Min-heap based on the K-th most recent access time (oldest = highest priority for eviction)
type HotHeap struct {
	entries []*ChunkEntry
	k       int
}

func NewHotHeap(k int) *HotHeap {
	return &HotHeap{
		entries: make([]*ChunkEntry, 0),
		k:       k,
	}
}

func (h *HotHeap) Len() int { return len(h.entries) }

func (h *HotHeap) Less(i, j int) bool {
	// Min-heap: entry with older K-th access time has higher priority (comes first)
	iAccessTimes := h.entries[i].accessTimes
	jAccessTimes := h.entries[j].accessTimes
	iTime := iAccessTimes[len(iAccessTimes)-h.k]
	jTime := jAccessTimes[len(jAccessTimes)-h.k]
	return iTime.Before(jTime)
}

func (h *HotHeap) Swap(i, j int) {
	h.entries[i], h.entries[j] = h.entries[j], h.entries[i]
	h.entries[i].heapIndex = i
	h.entries[j].heapIndex = j
}

func (h *HotHeap) Push(x interface{}) {
	entry := x.(*ChunkEntry)
	entry.heapIndex = len(h.entries)
	h.entries = append(h.entries, entry)
}

func (h *HotHeap) Pop() interface{} {
	old := h.entries
	n := len(old)
	entry := old[n-1]
	old[n-1] = nil // avoid memory leak
	entry.heapIndex = -1
	h.entries = old[0 : n-1]
	return entry
}

// Peek returns the entry with the oldest K-th access time without removing it
func (h *HotHeap) Peek() *ChunkEntry {
	if len(h.entries) == 0 {
		return nil
	}
	return h.entries[0]
}

// Update re-establishes heap ordering after an entry's access times have changed
func (h *HotHeap) Update(entry *ChunkEntry) {
	if entry.heapIndex >= 0 && entry.heapIndex < len(h.entries) {
		heap.Fix(h, entry.heapIndex)
	}
}

// Remove removes a specific entry from the heap
func (h *HotHeap) Remove(entry *ChunkEntry) {
	if entry.heapIndex >= 0 && entry.heapIndex < len(h.entries) {
		heap.Remove(h, entry.heapIndex)
	}
}

type ChunkStats struct {
	Hits  int64
	Calls int64
}

type ChunkRegistry struct {
	deletionLock        sync.RWMutex // freezes the cache when evictor runs
	statsLock           sync.Mutex   // protect hotList and coldList and other "stats": only one goroutine can AddAccess at the same time
	chunkLocks          sync.Map     // protects individual chunks. only one thread can access or change chunk files at the same time
	deleteLoopScheduled bool

	snpMgr   *SnapshotManager
	K        int
	capacity uint64
	hotHeap  *HotHeap // min-heap ordered by K-th most recent access time
	coldList *list.List
	items    sync.Map
	stats    sync.Map

	accessHistory []string
	historyLock   sync.Mutex

	deleteBatchSize uint64 // Threshold for evictor to start deleting chunks
}

func NewChunkRegistry(snpMgr *SnapshotManager, K int) *ChunkRegistry {
	cr := &ChunkRegistry{
		snpMgr:              snpMgr,
		K:                   K,
		capacity:            snpMgr.cacheSize,
		hotHeap:             NewHotHeap(K),
		coldList:            list.New(),
		deleteBatchSize:     deleteBatchSize,
		deleteLoopScheduled: false,
	}
	heap.Init(cr.hotHeap)
	return cr
}

// should only be called while holding the chunk's lock, otherwise might return true while chunk is being deleted
func (cr *ChunkRegistry) ChunkExists(hash string) bool {
	_, ok := cr.items.Load(hash)

	cr.historyLock.Lock()
	cr.accessHistory = append(cr.accessHistory, fmt.Sprintf("%v + %s", time.Now(), hash))
	cr.historyLock.Unlock()

	actualIface, _ := cr.stats.LoadOrStore(hash, &ChunkStats{})
	stats := actualIface.(*ChunkStats)
	atomic.AddInt64(&stats.Calls, 1)
	if ok {
		atomic.AddInt64(&stats.Hits, 1)
	}

	return ok
}

func (cr *ChunkRegistry) GetAccessHistory() []string {
	cr.historyLock.Lock()
	historyCopy := make([]string, len(cr.accessHistory))
	copy(historyCopy, cr.accessHistory)
	cr.historyLock.Unlock()
	return historyCopy
}

func (cr *ChunkRegistry) GetHitStats() map[string]ChunkStats {
	allStats := make(map[string]ChunkStats)

	cr.stats.Range(func(key, value interface{}) bool {
		hash := key.(string)
		statsPtr := value.(*ChunkStats)

		// Read the atomic values safely
		currentStats := ChunkStats{
			Calls: atomic.LoadInt64(&statsPtr.Calls),
			Hits:  atomic.LoadInt64(&statsPtr.Hits),
		}
		allStats[hash] = currentStats
		return true
	})

	return allStats
}

// Assumes caller holds hash's per-chunk lock and the RdeletionLock
func (cr *ChunkRegistry) AddAccess(hash string) error {
	cr.statsLock.Lock() // makes accesses to coldList and hotHeap and stats operations concurrency-safe
	defer cr.statsLock.Unlock()

	now := time.Now()

	entryIface, ok := cr.items.Load(hash)
	if !ok { // means the chunk is new
		entry := &ChunkEntry{
			hash:        hash,
			accessTimes: []time.Time{now},

			element:        nil,
			containingList: cr.coldList,
			heapIndex:      -1,
		}
		cr.items.Store(hash, entry)
		entry.element = cr.coldList.PushFront(entry)

		if cr.GetLength() > cr.capacity+cr.deleteBatchSize {
			if !cr.deleteLoopScheduled {
				cr.deleteLoopScheduled = true
				go cr.correctLength()
			}
		}

		return nil
	}

	entry := entryIface.(*ChunkEntry)
	entry.accessTimes = append(entry.accessTimes, now)

	if entry.containingList == cr.coldList {
		// Entry is in cold list
		if len(entry.accessTimes) == cr.K {
			// Promote to hot heap
			cr.coldList.Remove(entry.element)
			entry.element = nil
			entry.containingList = nil
			heap.Push(cr.hotHeap, entry)
		} else if len(entry.accessTimes) < cr.K {
			cr.coldList.MoveToFront(entry.element)
		} else {
			return errors.New(fmt.Sprintf("chunk %s is on cold list but has K or more accesses!", hash))
		}
	} else if entry.heapIndex >= 0 {
		// Entry is in hot heap, update its position after access time change
		cr.hotHeap.Update(entry)
	}
	return nil
}

// deletes extra chunk. assumes no lock.
func (cr *ChunkRegistry) correctLength() {
	start := time.Now()

	cr.deletionLock.Lock()
	defer cr.deletionLock.Unlock()
	cr.statsLock.Lock()
	defer cr.statsLock.Unlock()

	lockDuration := time.Since(start)

	firstLength := cr.GetLength()
	if firstLength > cr.capacity+cr.deleteBatchSize {
		count := 0
		for cr.GetLength() > cr.capacity {
			to_remove, err := cr.getVictimChunk()
			if err != nil {
				log.Errorf("error while getting the victim chunk")
				break
			}

			cr.historyLock.Lock()
			cr.accessHistory = append(cr.accessHistory, fmt.Sprintf("%v - %s", time.Now(), to_remove))
			cr.historyLock.Unlock()

			lockI, _ := cr.chunkLocks.LoadOrStore(to_remove, &sync.Mutex{})
			lock := lockI.(*sync.Mutex)
			lock.Lock()

			if err := cr.UnregisterChunk(to_remove); err != nil {
				log.Errorf("error while unregistering chunk %s: %v", to_remove, err)
				lock.Unlock()
				continue
			}

			if err := cr.snpMgr.RemoveChunk(to_remove); err != nil {
				log.Errorf("failed to remove chunk: %s, err: %v", to_remove, err)
				lock.Unlock()
				continue
			}

			count += 1

			lock.Unlock()
		}
		log.Debugf("deletionLoop ran (firstLength was %d). deleted %d chunks and took %v. Waited %v for locks", firstLength, count, time.Since(start), lockDuration)
	} else {
		log.Debugf("deletionLoop short-circuited (firstLength was %d). Waited %v for locks. returning in %v", firstLength, lockDuration, time.Since(start))
	}
	cr.deleteLoopScheduled = false
}

// assumes deletionLock and statsLock are held
func (cr *ChunkRegistry) getVictimChunk() (string, error) {
	// total := cr.GetLength()
	// selected := rand.Intn(total)
	// head := cr.coldList.Front()
	// for selected > 0 {
	// 	head = head.Next()
	// 	if head == nil {
	// 		head = cr.hotHeap.Peek()
	// 	}
	// 	selected -= 1
	// }
	// return head.Value.(*ChunkEntry).hash, nil

	var hotLRU, coldLRU *ChunkEntry = nil, nil

	// O(1) access to LRU entry in hot heap
	if cr.hotHeap.Len() > 0 {
		hotLRU = cr.hotHeap.Peek()
	}
	if cr.coldList.Len() > 0 {
		coldLRU = cr.coldList.Back().Value.(*ChunkEntry)
	}

	to_remove := ""
	if hotLRU == nil {
		to_remove = coldLRU.hash
	} else if coldLRU == nil {
		to_remove = hotLRU.hash
	} else {
		if int(time.Since(hotLRU.accessTimes[len(hotLRU.accessTimes)-1]).Milliseconds())/cr.K < int(time.Since(coldLRU.accessTimes[len(coldLRU.accessTimes)-1]).Milliseconds())/len(coldLRU.accessTimes) {
			to_remove = coldLRU.hash
		} else {
			to_remove = hotLRU.hash
		}
	}

	return to_remove, nil
}

// assumes caller holds lock for hash, the WdeletionLock, and statsLock; does NOT remove chunk from disk
func (cr *ChunkRegistry) UnregisterChunk(hash string) error {
	entryIface, ok := cr.items.Load(hash)
	if ok {
		entry := entryIface.(*ChunkEntry)
		if entry.containingList != nil {
			// Entry is in cold list
			if entry.containingList.Remove(entry.element) == nil {
				return errors.New(fmt.Sprintf("UnregisterChunk: chunk to delete (%s) not in coldList against expectation", hash))
			}
		} else if entry.heapIndex >= 0 {
			// Entry is in hot heap
			cr.hotHeap.Remove(entry)
		} else {
			return errors.New(fmt.Sprintf("UnregisterChunk: chunk (%s) not in any container", hash))
		}
		cr.items.Delete(hash)
		return nil
	} else {
		return errors.New(fmt.Sprintf("UnregisterChunk: chunk to delete (%s) not in registry", hash))
	}
}

// assumes statsLock is held
func (cr *ChunkRegistry) GetLength() uint64 {
	return uint64(cr.coldList.Len() + cr.hotHeap.Len())
}

// SnapshotManager manages snapshots stored on the node.
type SnapshotManager struct {
	sync.Mutex
	// Stored snapshots (identified by the function instance revision, which is provided by the `K_REVISION` environment
	// variable of knative).
	snapshots     map[string]*Snapshot
	baseFolder    string
	chunking      bool
	chunkRegistry *ChunkRegistry
	lazy          bool
	wsPulling     bool
	chunkSize     uint64
	cacheSize     uint64
	securityMode  string
	threads       int
	encryption    bool

	// Used to store remote snapshots
	storage storage.ObjectStorage
}

func readTarChunkHashes(tarFilePath string, chunkSize uint64) (map[[16]byte]bool, error) {
	file, err := os.Open(tarFilePath)
	if err != nil {
		log.Errorf("failed to open image file %s: %v", tarFilePath, err)
		return nil, err
	}
	defer file.Close()

	tr := tar.NewReader(file)
	chunks := make(map[[16]byte]bool)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break // End of tar archive
		}
		if err != nil {
			log.Errorf("error reading tar header for image %s: %v", tarFilePath, err)
			break
		}
		if hdr.Typeflag != tar.TypeReg {
			continue // Skip non-regular files
		}

		// Read the file content in chunks of chunkSize
		buffer := make([]byte, chunkSize)
		for {
			n, err := io.ReadFull(tr, buffer)
			if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
				log.Errorf("error reading file %s in tar %s: %v", hdr.Name, tarFilePath, err)
				break
			}
			if n == 0 {
				break
			}
			for i := n; i < len(buffer); i++ {
				buffer[i] = 0
			}

			hash := md5.Sum(buffer)
			chunks[hash] = true
		}
	}
	return chunks, nil
}

func NewSnapshotManager(baseFolder string, store storage.ObjectStorage, chunking, skipCleanup, lazy, wsPulling bool,
	chunkSize uint64, cacheSize uint64, securityMode string, threads int, encryption bool) *SnapshotManager {
	manager := &SnapshotManager{
		snapshots:     make(map[string]*Snapshot),
		baseFolder:    baseFolder,
		chunking:      chunking,
		chunkRegistry: nil, // TODO: tune params
		chunkSize:     chunkSize,
		storage:       store,
		wsPulling:     wsPulling,
		lazy:          lazy,
		cacheSize:     cacheSize,
		securityMode:  strings.ToLower(securityMode),
		threads:       threads,
		encryption:    encryption,
	}
	manager.chunkRegistry = NewChunkRegistry(manager, K)

	// Clean & init basefolder unless skipping is requested
	if !skipCleanup {
		_ = os.RemoveAll(manager.baseFolder)
	}
	_ = os.MkdirAll(manager.baseFolder, os.ModePerm)
	if chunking {
		_ = os.MkdirAll(filepath.Join(manager.baseFolder, chunkPrefix), os.ModePerm)
	}
	if skipCleanup {
		_ = manager.RecoverSnapshots()
	}

	go func() {
		if !chunking {
			return
		}

		imagesDir := filepath.Join(baseFolder, "..", "images")
		entries, err := os.ReadDir(imagesDir)
		if err != nil {
			log.Errorf("failed to read images directory: %v", err)
			return
		}
		for _, entry := range entries {
			if entry.IsDir() {
				image := entry.Name()
				imageChunks[image], _ = readTarChunkHashes(filepath.Join(imagesDir, image, "container.tar"), chunkSize)
				log.Debugf("Found image directory: %s", image)
			}
		}
		log.Infof("Loaded chunk hashes for %d images", len(imageChunks))

		rootfsChunks, _ = readTarChunkHashes(filepath.Join(imagesDir, "rootfs.tar"), chunkSize)
		log.Infof("Loaded rootfs chunk hashes, total %d chunks", len(rootfsChunks))
	}()

	return manager
}

func (mgr *SnapshotManager) PrepareBaseSnapshotChunks() {
	if len(baseSnapChunks) > 0 {
		return
	}
	if !mgr.chunking {
		return
	}

	baseSnapChunks = make(map[[16]byte]bool)
	baseSnap, err := os.Open(mgr.snapshots["base"].GetRecipeFilePath())
	if err != nil {
		log.Errorf("failed to open base snapshot recipe file: %v", err)
		return
	}
	defer baseSnap.Close()
	buffer := make([]byte, 16)
	for {
		n, err := baseSnap.Read(buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Errorf("error reading base snapshot recipe: %v", err)
			return
		}
		if n != 16 {
			log.Errorf("incomplete hash in base snapshot recipe")
			return
		}
		var hash [16]byte
		copy(hash[:], buffer)
		baseSnapChunks[hash] = true
	}

	log.Debugf("Base snapshot chunk hashes updated, total %d chunks", len(baseSnapChunks))
}

func (mgr *SnapshotManager) WriteHitStatsToCSV(filePath string) error {
	statsMap := mgr.chunkRegistry.GetHitStats()

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create CSV file %s: %w", filePath, err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"ChunkHash", "Calls", "Hits", "HitRate"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	for hash, stats := range statsMap {
		var hitRate float64
		if stats.Calls > 0 {
			hitRate = float64(stats.Hits) / float64(stats.Calls)
		} else {
			hitRate = 0.0
		}

		record := []string{
			hash,
			strconv.FormatInt(stats.Calls, 10),
			strconv.FormatInt(stats.Hits, 10),
			fmt.Sprintf("%.4f", hitRate),
		}

		if err := writer.Write(record); err != nil {
			return fmt.Errorf("failed to write record for hash %s: %w", hash, err)
		}
	}

	return nil
}

func (mgr *SnapshotManager) WriteAccessHistoryToTextFile(filePath string) error {
	accessList := mgr.chunkRegistry.GetAccessHistory()

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create text file %s: %w", filePath, err)
	}
	defer file.Close()

	for i, hash := range accessList {
		_, err := fmt.Fprintf(file, "%s\n", hash)

		if err != nil {
			return fmt.Errorf("failed to write hash at index %d (%s) to file: %w", i, hash, err)
		}
	}

	return nil
}

// RecoverSnapshots scans the base folder and recreates snapshot entries in the manager
// for any existing snapshots. This is used when skipCleanup is true to recover state
// after a restart.
func (mgr *SnapshotManager) RecoverSnapshots() error {
	logger := log.WithField("baseFolder", mgr.baseFolder)
	logger.Debug("Recovering snapshots from base folder")

	entries, err := os.ReadDir(mgr.baseFolder)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Base folder doesn't exist yet, nothing to recover
		}
		return errors.Wrapf(err, "reading base folder %s", mgr.baseFolder)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Skip the chunks directory
		if entry.Name() == chunkPrefix {
			continue
		}

		revision := entry.Name()
		snapDir := filepath.Join(mgr.baseFolder, revision)
		infoPath := filepath.Join(snapDir, "info_file")

		// Check if this looks like a valid snapshot directory
		if _, err := os.Stat(infoPath); os.IsNotExist(err) {
			logger.Warnf("Skipping directory %s: missing info_file", revision)
			continue
		}

		// Create snapshot object
		snap := NewSnapshot(revision, mgr.baseFolder, "")

		// Load snapshot info
		if err := snap.LoadSnapInfo(infoPath); err != nil {
			logger.Warnf("Failed to load snapshot info for %s: %v", revision, err)
			continue
		}

		// Mark as ready if all required files exist
		snap.ready = true
		mgr.snapshots[revision] = snap

		logger.Infof("Recovered snapshot for revision %s", revision)
	}

	prefixes, err := os.ReadDir(mgr.baseFolder + "/" + chunkPrefix)
	for _, prefix := range prefixes {
		chunks, err := os.ReadDir(mgr.baseFolder + "/" + chunkPrefix + "/" + prefix.Name())
		if err != nil {
			return err
		}
		for _, chunk := range chunks {
			mgr.chunkRegistry.AddAccess(chunk.Name())
		}
	}

	logger.Infof("Recovered %d snapshot(s), %d chunks", len(mgr.snapshots), mgr.chunkRegistry.GetLength())
	return nil
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

	// Return snapshot for supplied revision
	return mgr.snapshots[revision], nil
}

// InitSnapshot initializes a snapshot by adding its metadata to the SnapshotManager. Once the snapshot has
// been created, CommitSnapshot must be run to finalize the snapshot creation and make the snapshot available for use.
func (mgr *SnapshotManager) InitSnapshot(revision, image string) (*Snapshot, error) {
	mgr.Lock()

	logger := log.WithFields(log.Fields{"revision": revision, "image": image})
	logger.Debug("Initializing snapshot corresponding to revision and image")

	if snp, present := mgr.snapshots[revision]; present {
		ready := snp.ready
		mgr.Unlock()
		return snp, errors.New(fmt.Sprintf("Add: Snapshot for revision %s already exists and its ready is %v", revision, ready))
	}

	// Create snapshot object and move into creating state
	snap := NewSnapshot(revision, mgr.baseFolder, image)
	mgr.snapshots[snap.GetId()] = snap
	mgr.Unlock()

	// Create directory to store snapshot data
	err := snap.CreateSnapDir()
	if err != nil {
		return nil, errors.Wrapf(err, "creating snapDir for snapshots %s", revision)
	}

	return snap, nil
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

	snap.ready = true

	logger := log.WithFields(log.Fields{"revision": revision})
	logger.Debug("finished commiting snapshot " + revision)

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

func (mgr *SnapshotManager) CleanChunks() error {
	if !mgr.chunking {
		return nil
	}

	mgr.Lock()
	defer mgr.Unlock()

	mgr.chunkRegistry.deletionLock.Lock()
	defer mgr.chunkRegistry.deletionLock.Unlock()

	mgr.chunkRegistry.statsLock.Lock()
	defer mgr.chunkRegistry.statsLock.Unlock()

	hashes := []string{}

	mgr.chunkRegistry.items.Range(func(key, value interface{}) bool {
		hash := key.(string)
		entry := value.(*ChunkEntry)

		lockI, _ := mgr.chunkRegistry.chunkLocks.LoadOrStore(hash, &sync.Mutex{})
		lock := lockI.(*sync.Mutex)
		lock.Lock()
		defer lock.Unlock()

		hashes = append(hashes, entry.hash)
		return true
	})

	for _, hash := range hashes {
		os.Remove(mgr.GetChunkFilePath(hash))
	}

	mgr.chunkRegistry = NewChunkRegistry(mgr, K)
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
		snap.GetSnapshotFilePath(),
		snap.GetInfoFilePath(),
	}

	for _, filePath := range files {
		if err := mgr.uploadFile(revision, filePath); err != nil {
			return err
		}
	}

	err = mgr.uploadMemFile(snap)
	if err != nil {
		return errors.Wrapf(err, "uploading memory file for snapshot %s", revision)
	}

	return nil
}

func (mgr *SnapshotManager) UploadWSFile(revision string) error {
	snap, err := mgr.AcquireSnapshot(revision)
	if err != nil {
		return errors.Wrapf(err, "acquiring snapshot")
	}

	if err := mgr.uploadFile(revision, snap.GetWSFilePath()); err != nil {
		return errors.Wrapf(err, "uploading working set file for snapshot %s", revision)
	}

	return nil
}

func isHashSensitiveChunk(hash [16]byte, image string) bool {
	if image == "" { // case of base snapshot
		return false
	}

	lastSlash := strings.LastIndex(image, "/")
	if lastSlash != -1 {
		image = image[lastSlash+1:]
	}
	if colon := strings.Index(image, ":"); colon != -1 {
		image = image[:colon]
	}
	if ok, _ := imageChunks[image][hash]; ok {
		return false
	}

	if ok, _ := rootfsChunks[hash]; ok {
		return false
	}

	if ok, _ := baseSnapChunks[hash]; ok {
		return false
	}

	return true
}

func (mgr *SnapshotManager) uploadMemFile(snap *Snapshot) error {
	startTime := time.Now()
	log.Debugf("starting uploadMemFile for snapshot %s at %v", snap.id, startTime)
	if !mgr.chunking {
		error := mgr.uploadFile(snap.GetId(), snap.GetMemFilePath())
		log.Infof("unchunked uploadMemFile for snapshot %s completed in %s", snap.GetId(), time.Since(startTime))
		return error
	}

	file, err := os.Open(snap.GetMemFilePath())
	if err != nil {
		return errors.Wrapf(err, "opening memory file for chunked upload")
	}
	defer file.Close()

	type chunkJob struct {
		idx  int
		hash string
		data []byte
	}

	jobs := make(chan chunkJob, 128) // buffered channel, TODO: tune length
	errCh := make(chan error, 128)   // TODO: tune length
	var wg sync.WaitGroup

	for w := 0; w < mgr.threads; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				mgr.chunkRegistry.deletionLock.RLock()
				lockI, _ := mgr.chunkRegistry.chunkLocks.LoadOrStore(job.hash, &sync.Mutex{})
				lock := lockI.(*sync.Mutex)

				// start := time.Now()
				lock.Lock()
				// log.Debugf("uploadMemFile: Acquired lock for chunk %s in %v", job.hash, time.Since(start))

				if mgr.chunkRegistry.ChunkExists(job.hash) {
					lock.Unlock()
					mgr.chunkRegistry.deletionLock.RUnlock()
					continue
				}

				if found, err := mgr.storage.Exists(mgr.getObjectKey(chunkPrefix, job.hash)); err == nil && found {
					lock.Unlock()
					mgr.chunkRegistry.deletionLock.RUnlock()
					continue
				}

				chunkFilePath := mgr.GetChunkFilePath(job.hash)
				dir := filepath.Dir(chunkFilePath)
				if _, err := os.Stat(dir); os.IsNotExist(err) {
					os.MkdirAll(dir, os.ModePerm)
				}

				chunkFile, err := os.Create(chunkFilePath)
				if err != nil {
					errCh <- fmt.Errorf("creating chunk %s: %w", chunkFilePath, err)
					lock.Unlock()
					mgr.chunkRegistry.deletionLock.RUnlock()
					break
				}

				encryptedData := []byte{}
				if mgr.encryption {
					block, err := aes.NewCipher(EncryptionKey[:16])
					if err != nil {
						errCh <- fmt.Errorf("creating cipher for chunk: %w", err)
						lock.Unlock()
						mgr.chunkRegistry.deletionLock.RUnlock()
						break
					}
					stream := cipher.NewCTR(block, make([]byte, 16))
					encryptedData = make([]byte, len(job.data))
					stream.XORKeyStream(encryptedData, job.data)
				} else {
					encryptedData = job.data
				}

				if _, err := chunkFile.Write(encryptedData); err != nil {
					chunkFile.Close()
					errCh <- fmt.Errorf("writing chunk %d: %w", job.idx, err)
					lock.Unlock()
					mgr.chunkRegistry.deletionLock.RUnlock()
					break
				}
				chunkFile.Close()

				if err := mgr.uploadFile(chunkPrefix, chunkFilePath); err != nil {
					errCh <- fmt.Errorf("uploading chunk %d: %w", job.idx, err)
					lock.Unlock()
					mgr.chunkRegistry.deletionLock.RUnlock()
					continue
				}

				mgr.chunkRegistry.AddAccess(job.hash)
				lock.Unlock()
				mgr.chunkRegistry.deletionLock.RUnlock()
			}
		}()
	}

	buffer := make([]byte, mgr.chunkSize)
	chunkIndex := 0
	recipe := make([]byte, 0)

	// Sequential read & hash generation
	for {
		n, err := io.ReadFull(file, buffer)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return errors.Wrapf(err, "reading chunk %d from memory file", chunkIndex)
		}
		if n == 0 {
			break
		}

		hash := md5.Sum(buffer[:n])
		if mgr.securityMode == "full" {
			hash = md5.Sum(append(hash[:], []byte(snap.id)...))
		} else if mgr.securityMode == "partial" && isHashSensitiveChunk(hash, snap.Image) {
			hash = md5.Sum(append(hash[:], []byte(snap.id)...))
		}
		recipe = append(recipe, hash[:]...)
		chunkHash := hex.EncodeToString(hash[:])

		// Send job to worker
		dataCopy := make([]byte, n)
		copy(dataCopy, buffer[:n])
		jobs <- chunkJob{idx: chunkIndex, hash: chunkHash, data: dataCopy}

		chunkIndex++

		if err == io.EOF {
			break
		}
	}
	close(jobs)
	wg.Wait()
	close(errCh)

	// Check for errors
	var firstErr error
	for err := range errCh {
		log.Printf("Chunk upload error: %v", err) // print all errors
		if firstErr == nil {
			firstErr = err // remember first error to return
		}
	}

	if firstErr != nil {
		return firstErr
	}

	log.Infof("All chunks uploaded, preparing recipe file")
	recipeFilePath := snap.GetRecipeFilePath()
	log.Infof("Recipe file path: %s", recipeFilePath)

	recipeFile, err := os.Create(recipeFilePath)
	if err != nil {
		log.Errorf("Failed to create recipe file: %v", err)
		return errors.Wrapf(err, "creating recipe file for chunked upload")
	}
	defer func() {
		recipeFile.Close()
		log.Infof("Closed recipe file handle")
	}()

	log.Infof("Writing recipe file (%d bytes)", len(recipe))
	if _, err := recipeFile.Write(recipe); err != nil {
		log.Errorf("Failed to write recipe file: %v", err)
		return errors.Wrapf(err, "writing recipe file for chunked upload")
	}
	log.Infof("Recipe file written successfully")

	log.Infof("Uploading recipe file for snapshot %s", snap.GetId())
	if err := mgr.uploadFile(snap.GetId(), recipeFilePath); err != nil {
		log.Errorf("Failed to upload recipe file: %v", err)
		return err
	}
	log.Infof("Recipe file uploaded successfully")

	log.Infof("uploadMemFile for snapshot %s completed in %s, total chunks: %d", snap.GetId(), time.Since(startTime), chunkIndex)
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
		// snap.GetMemFilePath(),
	}
	for _, filePath := range files {
		fileName := filepath.Base(filePath)
		if err := mgr.downloadFile(revision, filePath, fileName); err != nil {
			return nil, errors.Wrapf(err, "downloading file %s", fileName)
		}
	}

	if found, err := mgr.storage.Exists(mgr.getObjectKey(snap.GetId(), filepath.Base(snap.GetWSFilePath()))); err == nil && found {
		// Download working set file, if it exists
		wsFilePath := snap.GetWSFilePath()
		wsFileName := filepath.Base(wsFilePath)
		if err := mgr.downloadFile(snap.GetId(), wsFilePath, wsFileName); err != nil {
			return nil, errors.Wrapf(err, "downloading working set file for lazy chunked download")
		}
		log.Infof("Downloaded working set file for snapshot %s", snap.GetId())
	}

	err = mgr.downloadMemFile(snap)
	if err != nil {
		return nil, errors.Wrapf(err, "downloading memory file for snapshot %s", revision)
	}
	log.Debug("Finished downloading memfile for revision " + revision)
	// stat, _ := os.Stat(snap.GetMemFilePath())
	// log.Infof("Downloaded memory file for snapshot %s, size is %d", snap.GetId(), stat.Size())

	if err := mgr.CommitSnapshot(revision); err != nil {
		return nil, errors.Wrap(err, "committing snapshot")
	}

	return snap, nil
}

func (mgr *SnapshotManager) downloadMemFile(snap *Snapshot) error {
	startTime := time.Now()

	if !mgr.chunking {
		error := mgr.downloadFile(snap.GetId(), snap.GetMemFilePath(), filepath.Base(snap.GetMemFilePath()))
		log.Infof("unchunked downloadMemFile for snapshot %s completed in %s", snap.GetId(), time.Since(startTime))
		return error
	}

	recipeFilePath := snap.GetRecipeFilePath()
	recipeFileName := filepath.Base(recipeFilePath)
	if err := mgr.downloadFile(snap.GetId(), recipeFilePath, recipeFileName); err != nil {
		return errors.Wrapf(err, "downloading recipe file for chunked download")
	}
	if mgr.lazy {
		if !mgr.wsPulling {
			return nil // nothing more to do in lazy mode without WS pulling
		}
		if stat, err := os.Stat(snap.GetWSFilePath()); err != nil || stat.Size() == 0 {
			log.Infof("No working set file for snapshot %s, skipping WS pulling", snap.GetId())
			return nil // nothing more to do if no working set file
		}

		return mgr.downloadWorkingSet(snap)
	}

	outFile, err := os.Create(snap.GetMemFilePath())
	if err != nil {
		return errors.Wrapf(err, "creating memory file for chunked download")
	}
	defer outFile.Close()

	// Worker pool
	type job struct {
		idx  int
		hash string
	}

	recipe, err := os.ReadFile(recipeFilePath)
	if err != nil {
		return errors.Wrapf(err, "reading recipe file")
	}

	// Extract chunk hashes
	var hashes []string
	for i := 0; i < len(recipe); i += md5.Size {
		if i+md5.Size > len(recipe) {
			break
		}
		hashes = append(hashes, hex.EncodeToString(recipe[i:i+md5.Size]))
	}

	var wg sync.WaitGroup
	jobs := make(chan job, len(hashes))

	for w := 0; w < mgr.threads; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				hash := j.hash
				idx := j.idx

				chunk_bytes, err := mgr.DownloadAndReturnChunk(hash)
				if err != nil {
					log.Printf("Error downloading chunk %d: %v", idx, err)
					continue
				}

				offset := int64(idx * int(mgr.chunkSize))
				if _, err := outFile.WriteAt(chunk_bytes, offset); err != nil {
					log.Printf("Error writing chunk %d: %v", idx, err)
				}
			}
		}()
	}

	for idx, hash := range hashes {
		jobs <- job{idx, hash}
	}
	close(jobs)

	wg.Wait()

	log.Infof("downloadMemFile for snapshot %s completed in %s, %d chunks downloaded", snap.GetId(), time.Since(startTime), len(hashes))
	return nil
}

func (mgr *SnapshotManager) DownloadAndReturnChunk(hash string) ([]byte, error) {
	mgr.chunkRegistry.deletionLock.RLock()
	defer mgr.chunkRegistry.deletionLock.RUnlock()

	lockI, _ := mgr.chunkRegistry.chunkLocks.LoadOrStore(hash, &sync.Mutex{})
	lock := lockI.(*sync.Mutex)

	lock.Lock()
	defer lock.Unlock()

	chunkFilePath := mgr.GetChunkFilePath(hash)

	// Return from in-memory registry if already downloaded
	if mgr.chunkRegistry.ChunkExists(hash) {
		data, err := os.ReadFile(chunkFilePath)
		if err != nil {
			return nil, errors.Wrapf(err, "reading cached chunk %s", hash)
		}
		mgr.chunkRegistry.AddAccess(hash)

		if mgr.encryption {
			block, err := aes.NewCipher(EncryptionKey[:16])
			if err != nil {
				return nil, fmt.Errorf("failed to create cipher: %w", err)
			}

			if len(data) < aes.BlockSize {
				return nil, fmt.Errorf("chunk content too short for IV")
			}

			stream := cipher.NewCTR(block, make([]byte, 16))
			stream.XORKeyStream(data, data)
		}

		return data, nil
	}

	// Download and store chunk
	objectKey := mgr.getObjectKey(chunkPrefix, hash)
	obj, err := mgr.storage.DownloadObject(objectKey)
	if err != nil {
		return nil, errors.Wrapf(err, "downloading chunk %s", hash)
	}
	defer obj.Close()

	dir := filepath.Dir(chunkFilePath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		os.MkdirAll(dir, os.ModePerm)
	}
	outFile, err := os.Create(chunkFilePath)
	if err != nil {
		return nil, errors.Wrapf(err, "creating chunk file %s", chunkFilePath)
	}
	defer outFile.Close()

	data, err := io.ReadAll(obj) // read object into memory
	if err != nil {
		return nil, errors.Wrapf(err, "reading chunk %s", hash)
	}

	if _, err := outFile.Write(data); err != nil {
		return nil, errors.Wrapf(err, "writing chunk %s", hash)
	}

	// Mark as downloaded
	mgr.chunkRegistry.AddAccess(hash)

	if mgr.encryption {
		block, err := aes.NewCipher(EncryptionKey[:16])
		if err != nil {
			return nil, fmt.Errorf("failed to create cipher: %w", err)
		}

		if len(data) < aes.BlockSize {
			return nil, fmt.Errorf("chunk content too short for IV")
		}

		stream := cipher.NewCTR(block, make([]byte, 16))
		stream.XORKeyStream(data, data)
	}

	return data, nil
}

func (mgr *SnapshotManager) DownloadChunk(hash string) error {
	mgr.chunkRegistry.deletionLock.RLock()
	defer mgr.chunkRegistry.deletionLock.RUnlock()

	lockI, _ := mgr.chunkRegistry.chunkLocks.LoadOrStore(hash, &sync.Mutex{})
	lock := lockI.(*sync.Mutex)

	lock.Lock()
	defer lock.Unlock()

	if mgr.chunkRegistry.ChunkExists(hash) {
		mgr.chunkRegistry.AddAccess(hash)
		return nil // already downloaded
	}
	chunkFilePath := mgr.GetChunkFilePath(hash)

	if err := mgr.downloadFile(chunkPrefix, chunkFilePath, hash); err != nil {
		return err
	}

	mgr.chunkRegistry.AddAccess(hash)
	return nil
}

// removes the chunk from local disk. assumes chunk lock and deletionLock are held
func (mgr *SnapshotManager) RemoveChunk(hash string) error {
	chunkFilePath := mgr.GetChunkFilePath(hash)

	// Check if file exists
	if _, err := os.Stat(chunkFilePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("chunk %s does not exist at %s", hash, chunkFilePath)
		}
		return fmt.Errorf("failed to stat chunk %s: %w", hash, err)
	}

	// Remove the file
	if err := os.Remove(chunkFilePath); err != nil {
		return fmt.Errorf("failed to remove chunk %s: %w", hash, err)
	}

	return nil
}

func (mgr *SnapshotManager) GetChunkFilePath(hash string) string {
	return filepath.Join(mgr.baseFolder, chunkPrefix, hash[:2], hash)
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

	dir := filepath.Dir(filePath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		os.MkdirAll(dir, os.ModePerm)
	}
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

func (mgr *SnapshotManager) downloadWorkingSet(snap *Snapshot) error {
	log.Debugf("start downloadWorkingSet for %s", snap.id)
	wsFile, err := os.Open(snap.GetWSFilePath())
	if err != nil {
		return errors.Wrapf(err, "opening working set file for lazy chunked download")
	}
	defer wsFile.Close()

	wsPages, err := io.ReadAll(wsFile)
	if err != nil {
		return errors.Wrapf(err, "reading working set file for lazy chunked download")
	}

	recipeFile, err := os.Open(snap.GetRecipeFilePath())
	if err != nil {
		return errors.Wrapf(err, "opening recipe file for lazy chunked download")
	}
	defer recipeFile.Close()

	recipe, err := io.ReadAll(recipeFile)
	if err != nil {
		return errors.Wrapf(err, "reading recipe file for lazy chunked download")
	}

	// Parse working set pages (skip first entry which is header/total count)
	lines := strings.Split(string(wsPages), "\n")
	if len(lines) <= 1 {
		return errors.New("working set file is empty or invalid")
	}

	chunksToLoad := make(map[string]bool)
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse page offset from working set
		var pageOffset uint64
		if _, err := fmt.Sscanf(line, "%d", &pageOffset); err != nil {
			continue // Skip invalid lines
		}

		// Calculate which chunk this page belongs to
		byteOffset := pageOffset * 4096 // Assuming 4KB pages
		chunkIndex := byteOffset / mgr.chunkSize

		// Get chunk hash from recipe
		hashStart := int(chunkIndex) * md5.Size
		hashEnd := hashStart + md5.Size
		if hashEnd > len(recipe) {
			continue // Page is beyond recipe bounds
		}

		hash := hex.EncodeToString(recipe[hashStart:hashEnd])
		chunksToLoad[hash] = true
	}

	var wg sync.WaitGroup
	jobs := make(chan string, len(chunksToLoad))

	for w := 0; w < mgr.threads; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for hash := range jobs {
				err := mgr.DownloadChunk(hash)
				if err != nil {
					log.Printf("Error downloading chunk %s: %v", hash, err)
					continue
				}
			}
		}()
	}

	for hash := range chunksToLoad {
		jobs <- hash
	}
	close(jobs)

	wg.Wait()
	log.Infof("Finished downloading working set for snapshot %s, %d chunks downloaded", snap.GetId(), len(chunksToLoad))

	return nil
}

// SnapshotExists checks if all required snapshot files exist in remote storage
func (mgr *SnapshotManager) SnapshotExists(revision string) (bool, error) {
	log.Infof("[SnapshotExists] Checking snapshot existence for revision=%s", revision)

	// Create a temporary snapshot to get the expected file names
	snap := NewSnapshot(revision, mgr.baseFolder, "")
	requiredFiles := []string{
		filepath.Base(snap.GetSnapshotFilePath()),
		filepath.Base(snap.GetInfoFilePath()),
	}

	log.Infof("[SnapshotExists] Required files for revision=%s: %v", revision, requiredFiles)

	// Check each file exists
	for _, fileName := range requiredFiles {
		objectKey := mgr.getObjectKey(revision, fileName)
		log.Infof("[SnapshotExists] Checking existence of object key=%s", objectKey)

		exists, err := mgr.storage.Exists(objectKey)
		if err != nil {
			log.Errorf("[SnapshotExists] Error checking file=%s for revision=%s: %v",
				fileName, revision, err)
			return false, errors.Wrapf(err, "checking if file %s exists for snapshot %s", fileName, revision)
		}

		if !exists {
			log.Warnf("[SnapshotExists] File missing: %s (objectKey=%s) for revision=%s",
				fileName, objectKey, revision)
			return false, nil
		}

		log.Infof("[SnapshotExists] File present: %s (objectKey=%s)", fileName, objectKey)
	}

	log.Infof("[SnapshotExists] All required snapshot files exist for revision=%s", revision)
	return true, nil
}

// Helper function to construct object keys (you may need to adjust this based on your key structure)
func (mgr *SnapshotManager) getObjectKey(revision, fileName string) string {
	if revision == chunkPrefix {
		return fmt.Sprintf("%s/%s/%s", revision, fileName[:2], fileName)
	}
	return fmt.Sprintf("%s/%s", revision, fileName)
}
