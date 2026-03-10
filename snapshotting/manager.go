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
	"bytes"
	"container/heap"
	"container/list"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/csv"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
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
	wsSharedPrefix  = "ws_shared"
	wsBaseRootfsKey = "base_rootfs"
	K               = 3   // TODO: tune
	deleteBatchSize = 150 // TODO: tune
)

var imageChunks = map[string]map[[16]byte]bool{}
var rootfsChunks = map[[16]byte]bool{}
var baseSnapChunks = map[[16]byte]bool{}
var EncryptionKey = []byte("vhive-snapshot-enc") // 16 bytes key for AES-128

type WorkingSetContentSource struct {
	Content []byte
	Index   []byte
}

type WorkingSetContentSources struct {
	BaseRootfs WorkingSetContentSource
	Image      WorkingSetContentSource
	Private    WorkingSetContentSource
}

func (mgr *SnapshotManager) GetCleanChunks() bool {
	return mgr.cleanChunks
}

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

			if err := cr.snpMgr.removeChunkFile(to_remove); err != nil {
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
	optimizeWS    bool
	chunkSize     uint64
	cacheSize     uint64
	securityMode  string
	threads       int
	encryption    bool
	cleanChunks   bool
	wsBaseCache   *WorkingSetContentSource
	wsImageCache  sync.Map // image name -> *WorkingSetContentSource

	// Used to store remote snapshots
	storage storage.ObjectStorage
	initWg  sync.WaitGroup
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

func NewSnapshotManager(baseFolder string, store storage.ObjectStorage, chunking, skipCleanup, lazy, wsPulling, optimizeWS bool,
	chunkSize uint64, cacheSize uint64, securityMode string, threads int, encryption, cleanChunks bool) *SnapshotManager {
	manager := &SnapshotManager{
		snapshots:     make(map[string]*Snapshot),
		baseFolder:    baseFolder,
		chunking:      chunking,
		chunkRegistry: nil, // TODO: tune params
		chunkSize:     chunkSize,
		storage:       store,
		wsPulling:     wsPulling,
		optimizeWS:    optimizeWS,
		lazy:          lazy,
		cacheSize:     cacheSize,
		securityMode:  strings.ToLower(securityMode),
		threads:       threads,
		encryption:    encryption,
		cleanChunks:   cleanChunks,
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

	manager.initWg.Add(1)
	go func() {
		defer manager.initWg.Done()
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

func (mgr *SnapshotManager) WaitForInit() {
	mgr.initWg.Wait()
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
		if base, ok := mgr.snapshots["base"]; ok {
			_ = mgr.uploadMemFile(base)
		}
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
	if err != nil {
		logger.Infof("Recovered %d snapshot(s) with no chunks", len(mgr.snapshots))
		return nil
	}
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

func (mgr *SnapshotManager) UpdateWorkingSet(revision string, diffFile string) error {
	snap, err := mgr.AcquireSnapshot(revision)
	if err != nil {
		return errors.Wrapf(err, "acquiring snapshot")
	}

	os.Remove(snap.GetWSFilePath())                    // remove old working set file if exists
	ws_pages_data, err := mgr.GetWorkingSetPages(snap) // get the most up-to-date one
	if err != nil {
		return errors.Wrapf(err, "getting working set pages for snapshot %s", revision)
	}

	reader := csv.NewReader(bytes.NewReader(ws_pages_data))
	records, err := reader.ReadAll()
	if err != nil {
		return errors.Wrapf(err, "parsing working set CSV for snapshot %s", revision)
	}
	if len(records) == 0 || len(records[0]) == 0 || records[0][0] != "pfn" {
		return fmt.Errorf("unexpected ws source index format: expected pfn header")
	}

	ws_pages := make([]uint64, 0, len(records))
	for i := 1; i < len(records); i++ {
		if len(records[i]) == 0 {
			continue
		}
		pfn, err := strconv.ParseUint(records[i][0], 10, 64)
		if err != nil {
			continue
		}
		ws_pages = append(ws_pages, pfn)
	}

	df, err := os.Open(diffFile)
	if err != nil {
		return errors.Wrapf(err, "opening working set diff file for snapshot %s", revision)
	}
	defer df.Close()
	diffReader := csv.NewReader(io.Reader(df))
	records, err = diffReader.ReadAll()
	if err != nil {
		return errors.Wrapf(err, "parsing working set CSV for snapshot %s", revision)
	}
	if len(records) == 0 || len(records[0]) == 0 || records[0][0] != "pfn" {
		return fmt.Errorf("unexpected ws source index format: expected pfn header")
	}

	cnt := 0
	for i := 1; i < len(records); i++ {
		if len(records[i]) == 0 {
			continue
		}
		pfn, err := strconv.ParseUint(records[i][0], 10, 64)
		if err != nil {
			continue
		}

		if !slices.Contains(ws_pages, pfn) {
			cnt++
			ws_pages = append(ws_pages, pfn)
		}
	}
	log.Debugf("UpdateWorkingSet: %d new pages added to working set of snapshot %s", cnt, revision)

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{"pfn"}); err != nil {
		return errors.Wrapf(err, "writing header to working set CSV for snapshot %s", revision)
	}
	for _, pfn := range ws_pages {
		if err := writer.Write([]string{strconv.FormatUint(pfn, 10)}); err != nil {
			return errors.Wrapf(err, "writing record to working set CSV for snapshot %s", revision)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return errors.Wrapf(err, "finalizing working set CSV for snapshot %s", revision)
	}

	if err := os.WriteFile(snap.GetWSFilePath(), buf.Bytes(), 0644); err != nil {
		return errors.Wrapf(err, "writing updated working set CSV for snapshot %s", revision)
	}

	return mgr.UploadWorkingSet(revision)
}

func (mgr *SnapshotManager) UploadWorkingSet(revision string) error {
	if revision == "base" {
		return nil // base snapshot's working set is not uploaded since it is not used as a base for other snapshots
	}
	snap, err := mgr.AcquireSnapshot(revision)
	if err != nil {
		return errors.Wrapf(err, "acquiring snapshot")
	}

	if err := mgr.uploadFile(revision, snap.GetWSFilePath()); err != nil {
		return errors.Wrapf(err, "uploading working set file for snapshot %s", revision)
	}

	if mgr.optimizeWS && mgr.chunkSize == 4096 {
		wsFile, err := os.Open(snap.GetWSFilePath())
		if err != nil {
			return errors.Wrapf(err, "opening working set file")
		}
		defer wsFile.Close()

		var memFile *os.File
		useChunks := false

		if _, err := os.Stat(snap.GetMemFilePath()); err == nil {
			memFile, err = os.Open(snap.GetMemFilePath())
			if err != nil {
				return errors.Wrapf(err, "opening memory file")
			}
			defer memFile.Close()
		} else {
			useChunks = true
		}

		var recipe []byte
		if useChunks {
			recipe, err = os.ReadFile(snap.GetRecipeFilePath())
			if err != nil {
				return errors.Wrapf(err, "reading recipe file")
			}
		}

		reader := csv.NewReader(wsFile)
		records, err := reader.ReadAll()
		if err != nil {
			return errors.Wrapf(err, "reading working set CSV")
		}

		if mgr.securityMode == "full" {
			contentPath := snap.GetWSContentFilePath()
			contentFile, err := os.Create(contentPath)
			if err != nil {
				return errors.Wrapf(err, "creating monolithic working set content file")
			}
			defer contentFile.Close()

			page := make([]byte, 4096)
			for i := 1; i < len(records); i++ {
				record := records[i]
				if len(record) == 0 {
					continue
				}
				pfn, parseErr := strconv.ParseUint(record[0], 10, 64)
				if parseErr != nil {
					continue
				}

				if !useChunks {
					offset := int64(pfn * 4096)
					if _, err := memFile.Seek(offset, 0); err != nil {
						return errors.Wrapf(err, "seeking memory file")
					}
					if _, err := io.ReadFull(memFile, page); err != nil {
						return errors.Wrapf(err, "reading page from memory file")
					}
				} else {
					for j := range page {
						page[j] = 0
					}

					chunkIndex := int(pfn)
					hashStart := chunkIndex * md5.Size
					hashEnd := hashStart + md5.Size

					if hashEnd > len(recipe) {
						log.Warnf("Page %d (index %d) is beyond recipe size", pfn, chunkIndex)
					} else {
						hash := hex.EncodeToString(recipe[hashStart:hashEnd])
						data, dlErr := mgr.DownloadAndReturnChunk(hash)
						if dlErr != nil {
							return errors.Wrapf(dlErr, "downloading chunk %s", hash)
						}
						copy(page, data)
					}
				}

				if _, err := contentFile.Write(page); err != nil {
					return errors.Wrapf(err, "writing page to monolithic working set content file")
				}
			}

			if err := mgr.uploadFile(revision, contentPath); err != nil {
				return errors.Wrapf(err, "uploading monolithic working set content file")
			}

			return nil
		}

		type wsSourceBuild struct {
			pfns    []uint64
			content []byte
		}

		type wsSharedSourceBuild struct {
			hashes  []string
			content []byte
			seen    map[string]bool
		}

		appendPage := func(builder *wsSourceBuild, pfn uint64, page []byte) {
			builder.pfns = append(builder.pfns, pfn)
			builder.content = append(builder.content, page...)
		}

		appendSharedPage := func(builder *wsSharedSourceBuild, hash [md5.Size]byte, page []byte) {
			hashStr := hex.EncodeToString(hash[:])
			if builder.seen[hashStr] {
				return
			}
			builder.seen[hashStr] = true
			builder.hashes = append(builder.hashes, hashStr)
			builder.content = append(builder.content, page...)
		}

		baseBuild := &wsSharedSourceBuild{seen: make(map[string]bool)}
		imageBuild := &wsSharedSourceBuild{seen: make(map[string]bool)}
		privateBuild := &wsSourceBuild{}

		imageName := normalizeImageName(snap.GetImage())
		page := make([]byte, 4096)

		for i := 1; i < len(records); i++ {
			record := records[i]
			if len(record) == 0 {
				continue
			}
			pfn, parseErr := strconv.ParseUint(record[0], 10, 64)
			if parseErr != nil {
				continue
			}

			if !useChunks {
				offset := int64(pfn * 4096)
				if _, err := memFile.Seek(offset, 0); err != nil {
					return errors.Wrapf(err, "seeking memory file")
				}
				if _, err := io.ReadFull(memFile, page); err != nil {
					return errors.Wrapf(err, "reading page from memory file")
				}
			} else {
				for j := range page {
					page[j] = 0
				}

				chunkIndex := int(pfn)
				hashStart := chunkIndex * md5.Size
				hashEnd := hashStart + md5.Size

				if hashEnd > len(recipe) {
					log.Warnf("Page %d (index %d) is beyond recipe size", pfn, chunkIndex)
				} else {
					hash := hex.EncodeToString(recipe[hashStart:hashEnd])
					data, dlErr := mgr.DownloadAndReturnChunk(hash)
					if dlErr != nil {
						return errors.Wrapf(dlErr, "downloading chunk %s", hash)
					}
					copy(page, data)
				}
			}

			pageCopy := make([]byte, 4096)
			copy(pageCopy, page)
			hash := md5.Sum(pageCopy)

			if mgr.isBaseRootfsChunk(hash) {
				appendSharedPage(baseBuild, hash, pageCopy)
			} else if mgr.isImageChunk(hash, imageName) {
				appendSharedPage(imageBuild, hash, pageCopy)
			} else {
				appendPage(privateBuild, pfn, pageCopy)
			}
		}

		privateIndex, err := buildWSPFNIndexCSV(privateBuild.pfns)
		if err != nil {
			return errors.Wrapf(err, "building private working set index")
		}
		if err := os.WriteFile(snap.GetWSPrivateContentFilePath(), privateBuild.content, 0644); err != nil {
			return errors.Wrapf(err, "writing private working set content file")
		}
		if err := os.WriteFile(snap.GetWSPrivateIndexFilePath(), privateIndex, 0644); err != nil {
			return errors.Wrapf(err, "writing private working set index file")
		}
		if err := mgr.uploadFile(revision, snap.GetWSPrivateContentFilePath()); err != nil {
			return errors.Wrapf(err, "uploading private working set content file")
		}
		if err := mgr.uploadFile(revision, snap.GetWSPrivateIndexFilePath()); err != nil {
			return errors.Wrapf(err, "uploading private working set index file")
		}

		if len(baseBuild.hashes) > 0 {
			baseIndex, idxErr := buildWSHashIndexCSV(baseBuild.hashes)
			if idxErr != nil {
				return errors.Wrapf(idxErr, "building base/rootfs working set index")
			}
			if err := mgr.persistSharedWSSource("", baseBuild.content, baseIndex); err != nil {
				return errors.Wrapf(err, "persisting base/rootfs shared working set source")
			}
		}

		if len(imageBuild.hashes) > 0 && imageName != "" {
			imageIndex, idxErr := buildWSHashIndexCSV(imageBuild.hashes)
			if idxErr != nil {
				return errors.Wrapf(idxErr, "building image working set index")
			}
			if err := mgr.persistSharedWSSource(imageName, imageBuild.content, imageIndex); err != nil {
				return errors.Wrapf(err, "persisting image shared working set source")
			}
		}
	}

	return nil
}

func normalizeImageName(imageName string) string {
	imageName = strings.TrimSpace(imageName)
	if imageName == "" {
		return ""
	}

	if at := strings.Index(imageName, "@"); at != -1 {
		imageName = imageName[:at]
	}

	lastSlash := strings.LastIndex(imageName, "/")
	lastColon := strings.LastIndex(imageName, ":")
	if lastColon > lastSlash {
		imageName = imageName[:lastColon]
	}

	if idx := strings.LastIndex(imageName, "/"); idx != -1 {
		return imageName[idx+1:]
	}
	return imageName
}

func buildWSPFNIndexCSV(pfns []uint64) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{"pfn"}); err != nil {
		return nil, err
	}
	for _, pfn := range pfns {
		if err := writer.Write([]string{strconv.FormatUint(pfn, 10)}); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildWSHashIndexCSV(hashes []string) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{"hash"}); err != nil {
		return nil, err
	}
	for _, hash := range hashes {
		if err := writer.Write([]string{hash}); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (mgr *SnapshotManager) isBaseRootfsChunk(hash [16]byte) bool {
	if ok, _ := rootfsChunks[hash]; ok {
		return true
	}
	if ok, _ := baseSnapChunks[hash]; ok {
		return true
	}
	return false
}

func (mgr *SnapshotManager) isImageChunk(hash [16]byte, imageName string) bool {
	if imageName == "" {
		return false
	}
	hashes, ok := imageChunks[imageName]
	if !ok {
		return false
	}
	_, hit := hashes[hash]
	return hit
}

func (mgr *SnapshotManager) getSharedWSLocalPaths(imageName string) (string, string) {
	if imageName == "" {
		base := filepath.Join(mgr.baseFolder, wsSharedPrefix, wsBaseRootfsKey)
		return base + "_content", base + "_index"
	}
	base := filepath.Join(mgr.baseFolder, wsSharedPrefix, "images", imageName)
	return base + "_content", base + "_index"
}

func (mgr *SnapshotManager) getSharedWSObjectKeys(imageName string) (string, string) {
	if imageName == "" {
		return fmt.Sprintf("%s/%s/content", wsSharedPrefix, wsBaseRootfsKey), fmt.Sprintf("%s/%s/index", wsSharedPrefix, wsBaseRootfsKey)
	}
	return fmt.Sprintf("%s/images/%s/content", wsSharedPrefix, imageName), fmt.Sprintf("%s/images/%s/index", wsSharedPrefix, imageName)
}

func (mgr *SnapshotManager) getSharedWSChunkLocalDir(imageName string) string {
	if imageName == "" {
		return filepath.Join(mgr.baseFolder, wsSharedPrefix, wsBaseRootfsKey, "chunks")
	}
	return filepath.Join(mgr.baseFolder, wsSharedPrefix, "images", imageName, "chunks")
}

func (mgr *SnapshotManager) getSharedWSChunkObjectPrefix(imageName string) string {
	if imageName == "" {
		return fmt.Sprintf("%s/%s/chunks", wsSharedPrefix, wsBaseRootfsKey)
	}
	return fmt.Sprintf("%s/images/%s/chunks", wsSharedPrefix, imageName)
}

func parseWSHashIndex(index []byte) ([]string, error) {
	reader := csv.NewReader(bytes.NewReader(index))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 || len(records[0]) == 0 || records[0][0] != "hash" {
		return nil, fmt.Errorf("shared index is not hash-indexed")
	}

	hashes := make([]string, 0, len(records)-1)
	for i := 1; i < len(records); i++ {
		if len(records[i]) == 0 || records[i][0] == "" {
			continue
		}
		hashes = append(hashes, records[i][0])
	}

	return hashes, nil
}

func (mgr *SnapshotManager) persistSharedWSSource(imageName string, content []byte, index []byte) error {
	hashes, err := parseWSHashIndex(index)
	if err != nil {
		return errors.Wrapf(err, "parsing shared working set index")
	}
	if len(hashes) == 0 {
		return nil
	}

	if len(content) < len(hashes)*4096 {
		return errors.Errorf("shared content shorter than expected: have %d, expected at least %d", len(content), len(hashes)*4096)
	}

	localDir := mgr.getSharedWSChunkLocalDir(imageName)
	if err := os.MkdirAll(localDir, os.ModePerm); err != nil {
		return err
	}

	objectPrefix := mgr.getSharedWSChunkObjectPrefix(imageName)
	for i, hash := range hashes {
		start := i * 4096
		end := start + 4096
		page := content[start:end]

		localPath := filepath.Join(localDir, hash)
		if _, statErr := os.Stat(localPath); os.IsNotExist(statErr) {
			if writeErr := os.WriteFile(localPath, page, 0644); writeErr != nil {
				return errors.Wrapf(writeErr, "writing shared chunk %s locally", hash)
			}
		}

		if mgr.storage != nil {
			objectKey := fmt.Sprintf("%s/%s", objectPrefix, hash)
			exists, existsErr := mgr.storage.Exists(objectKey)
			if existsErr != nil {
				return errors.Wrapf(existsErr, "checking shared chunk object %s", objectKey)
			}
			if !exists {
				if upErr := mgr.storage.UploadObject(objectKey, bytes.NewReader(page), int64(len(page))); upErr != nil {
					return errors.Wrapf(upErr, "uploading shared chunk object %s", objectKey)
				}
			}
		}
	}

	if imageName == "" {
		mgr.wsBaseCache = nil
	} else {
		mgr.wsImageCache.Delete(imageName)
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

func (mgr *SnapshotManager) DeriveChunkHash(chunk []byte, revision, image string) [16]byte {
	hash := md5.Sum(chunk)
	if mgr.securityMode == "full" {
		return md5.Sum(append(hash[:], []byte(revision)...))
	}
	if mgr.securityMode == "partial" && isHashSensitiveChunk(hash, image) {
		return md5.Sum(append(hash[:], []byte(revision)...))
	}
	return hash
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

	recipe, chunkIndex, err := mgr.uploadChunkedMemoryContent(file, snap.id, snap.Image)
	if err != nil {
		return err
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

func (mgr *SnapshotManager) uploadChunkedMemoryContent(reader io.Reader, revision, image string) ([]byte, int, error) {
	if mgr.chunkSize == 0 {
		return nil, 0, errors.New("chunkSize must be greater than 0")
	}

	workerCount := mgr.threads
	if workerCount < 1 {
		workerCount = 1
	}

	type chunkJob struct {
		idx  int
		hash string
		data []byte
	}

	jobs := make(chan chunkJob, 128)
	errCh := make(chan error, 128)
	var wg sync.WaitGroup

	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				mgr.chunkRegistry.deletionLock.RLock()
				lockI, _ := mgr.chunkRegistry.chunkLocks.LoadOrStore(job.hash, &sync.Mutex{})
				lock := lockI.(*sync.Mutex)
				lock.Lock()

				if mgr.chunkRegistry.ChunkExists(job.hash) {
					lock.Unlock()
					mgr.chunkRegistry.deletionLock.RUnlock()
					continue
				}

				if found, existsErr := mgr.storage.Exists(mgr.getObjectKey(chunkPrefix, job.hash)); existsErr == nil && found {
					lock.Unlock()
					mgr.chunkRegistry.deletionLock.RUnlock()
					continue
				} else if existsErr != nil {
					errCh <- fmt.Errorf("checking chunk existence %s: %w", job.hash, existsErr)
					lock.Unlock()
					mgr.chunkRegistry.deletionLock.RUnlock()
					continue
				}

				chunkFilePath := mgr.GetChunkFilePath(job.hash)
				dir := filepath.Dir(chunkFilePath)
				if mkErr := os.MkdirAll(dir, os.ModePerm); mkErr != nil {
					errCh <- fmt.Errorf("creating chunk dir %s: %w", dir, mkErr)
					lock.Unlock()
					mgr.chunkRegistry.deletionLock.RUnlock()
					continue
				}

				chunkFile, createErr := os.Create(chunkFilePath)
				if createErr != nil {
					errCh <- fmt.Errorf("creating chunk %s: %w", chunkFilePath, createErr)
					lock.Unlock()
					mgr.chunkRegistry.deletionLock.RUnlock()
					continue
				}

				toWrite := job.data
				if mgr.encryption {
					encryptedData := make([]byte, len(job.data))
					if _, encErr := EncryptData(job.data, encryptedData, EncryptionKey[:16]); encErr != nil {
						chunkFile.Close()
						errCh <- fmt.Errorf("encrypting chunk %s: %w", job.hash, encErr)
						lock.Unlock()
						mgr.chunkRegistry.deletionLock.RUnlock()
						continue
					}
					toWrite = encryptedData
				}

				if _, writeErr := chunkFile.Write(toWrite); writeErr != nil {
					chunkFile.Close()
					errCh <- fmt.Errorf("writing chunk %d: %w", job.idx, writeErr)
					lock.Unlock()
					mgr.chunkRegistry.deletionLock.RUnlock()
					continue
				}
				chunkFile.Close()

				if uploadErr := mgr.uploadFile(chunkPrefix, chunkFilePath); uploadErr != nil {
					errCh <- fmt.Errorf("uploading chunk %d: %w", job.idx, uploadErr)
					lock.Unlock()
					mgr.chunkRegistry.deletionLock.RUnlock()
					continue
				}

				_ = mgr.chunkRegistry.AddAccess(job.hash)
				lock.Unlock()
				mgr.chunkRegistry.deletionLock.RUnlock()
			}
		}()
	}

	buffer := make([]byte, mgr.chunkSize)
	chunkIndex := 0
	recipe := make([]byte, 0)

	for {
		n, readErr := io.ReadFull(reader, buffer)
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			close(jobs)
			wg.Wait()
			close(errCh)
			return nil, chunkIndex, errors.Wrapf(readErr, "reading chunk %d", chunkIndex)
		}
		if n == 0 {
			break
		}

		hash := mgr.DeriveChunkHash(buffer[:n], revision, image)
		recipe = append(recipe, hash[:]...)
		chunkHash := hex.EncodeToString(hash[:])

		dataCopy := make([]byte, n)
		copy(dataCopy, buffer[:n])
		jobs <- chunkJob{idx: chunkIndex, hash: chunkHash, data: dataCopy}

		chunkIndex++
		if readErr == io.EOF {
			break
		}
	}

	close(jobs)
	wg.Wait()
	close(errCh)

	var firstErr error
	for err := range errCh {
		log.Printf("Chunk upload error: %v", err)
		if firstErr == nil {
			firstErr = err
		}
	}

	if firstErr != nil {
		return nil, chunkIndex, firstErr
	}

	return recipe, chunkIndex, nil
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

	infoPath := snap.GetInfoFilePath()
	type downloadJob struct {
		name string
		run  func() error
	}

	jobs := []downloadJob{
		{
			name: "manifest",
			run: func() error {
				return mgr.downloadFile(revision, infoPath, filepath.Base(infoPath))
			},
		},
		{
			name: "snapshot",
			run: func() error {
				snapshotPath := snap.GetSnapshotFilePath()
				return mgr.downloadFile(revision, snapshotPath, filepath.Base(snapshotPath))
			},
		},
	}

	if !mgr.lazy && revision != "base" {
		jobs = append(jobs, downloadJob{
			name: "memory",
			run: func() error {
				return mgr.downloadMemFile(snap)
			},
		})
	}

	if revision == "base" && mgr.chunking {
		jobs = append(jobs, downloadJob{
			name: "recipe",
			run: func() error {
				recipePath := snap.GetRecipeFilePath()
				return mgr.downloadFile(revision, recipePath, filepath.Base(recipePath))
			},
		})
	}

	errCh := make(chan error, len(jobs))
	var wg sync.WaitGroup
	for _, job := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if jobErr := job.run(); jobErr != nil {
				errCh <- errors.Wrapf(jobErr, "downloading %s for snapshot %s", job.name, revision)
			}
		}()
	}
	wg.Wait()
	close(errCh)

	for downloadErr := range errCh {
		if downloadErr != nil {
			return nil, downloadErr
		}
	}

	if err := snap.LoadSnapInfo(infoPath); err != nil {
		return nil, errors.Wrapf(err, "loading manifest from %s", infoPath)
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
		if err == nil {

			mgr.chunkRegistry.AddAccess(hash)

			if mgr.encryption {
				EncryptData(data, data, EncryptionKey[:16])
			}

			return data, nil
		}
		// Fallback to download if reading fails (e.g. file deleted)
	}

	// Download and store chunk
	objectKey := mgr.getObjectKey(chunkPrefix, hash)

	data, err := mgr.storage.DownloadObject(objectKey)
	if err != nil {
		return nil, errors.Wrapf(err, "downloading chunk %s", hash)
	}

	if !mgr.cleanChunks {
		// Write to file in background
		go func(data []byte, hash string) {
			// Acquire lock for this specific chunk since we're writing to disk/updating registry
			mgr.chunkRegistry.deletionLock.RLock()
			defer mgr.chunkRegistry.deletionLock.RUnlock()

			lockI, _ := mgr.chunkRegistry.chunkLocks.LoadOrStore(hash, &sync.Mutex{})
			lock := lockI.(*sync.Mutex)
			lock.Lock()
			defer lock.Unlock()

			// Double check if already exists/written by another thread
			if mgr.chunkRegistry.ChunkExists(hash) {
				return
			}

			chunkFilePath := mgr.GetChunkFilePath(hash)
			dir := filepath.Dir(chunkFilePath)
			if err = os.MkdirAll(dir, os.ModePerm); err != nil {
				log.Errorf("creating chunk directory %s: %v", dir, err)
				return
			}

			if err := os.WriteFile(chunkFilePath, data, 0644); err != nil {
				log.Errorf("writing chunk file %s: %v", hash, err)
				return
			}

			// Mark as downloaded
			mgr.chunkRegistry.AddAccess(hash)
		}(data, hash)
	}

	if mgr.encryption {
		EncryptData(data, data, EncryptionKey[:16])
	}

	return data, nil
}

func EncryptData(data, out []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	if len(data) < aes.BlockSize {
		return nil, fmt.Errorf("chunk content too short for IV")
	}

	stream := cipher.NewCTR(block, make([]byte, 16))
	stream.XORKeyStream(out, data)
	return out, nil
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

// RemoveChunk safely removes the chunk from the registry and disk.
func (mgr *SnapshotManager) RemoveChunk(hash string) error {
	mgr.chunkRegistry.deletionLock.Lock()
	defer mgr.chunkRegistry.deletionLock.Unlock()

	mgr.chunkRegistry.statsLock.Lock()
	defer mgr.chunkRegistry.statsLock.Unlock()

	lockI, _ := mgr.chunkRegistry.chunkLocks.LoadOrStore(hash, &sync.Mutex{})
	lock := lockI.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	if err := mgr.chunkRegistry.UnregisterChunk(hash); err != nil {
		log.Warnf("RemoveChunk: UnregisterChunk failed for %s: %v", hash, err)
	}

	return mgr.removeChunkFile(hash)
}

// removes the chunk from local disk. assumes chunk lock and deletionLock are held
func (mgr *SnapshotManager) removeChunkFile(hash string) error {
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
	objectKey := mgr.getObjectKey(revision, filepath.Base(filePath))
	return mgr.storage.UploadFile(objectKey, filePath)
}

// downloadFile Downloads a file from MinIO and save it to the specified path
func (mgr *SnapshotManager) downloadFile(revision, filePath, fileName string) error {
	objectKey := mgr.getObjectKey(revision, fileName)

	dir := filepath.Dir(filePath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		os.MkdirAll(dir, os.ModePerm)
	}

	return mgr.storage.DownloadFile(objectKey, filePath)
}

func (mgr *SnapshotManager) GetWorkingSetContent(snap *Snapshot) ([]byte, error) {
	if mgr.securityMode == "full" {
		return mgr.GetSnapshotFileContent(snap, snap.GetWSContentFilePath())
	}

	data, err := mgr.GetSnapshotFileContent(snap, snap.GetWSPrivateContentFilePath())
	if err == nil {
		return data, nil
	}
	return mgr.GetSnapshotFileContent(snap, snap.GetWSContentFilePath())
}

func (mgr *SnapshotManager) GetWorkingSetContentSources(snap *Snapshot) (*WorkingSetContentSources, error) {
	if mgr.securityMode == "full" {
		return nil, nil
	}

	privateContent, err := mgr.GetWorkingSetContent(snap)
	if err != nil {
		privateContent = nil
	}

	privateIndex, err := mgr.GetSnapshotFileContent(snap, snap.GetWSPrivateIndexFilePath())
	if err != nil {
		privateIndex = nil
	}

	baseSource, err := mgr.getSharedWSSource("")
	if err != nil {
		return nil, err
	}

	imageSource, err := mgr.getSharedWSSource(normalizeImageName(snap.GetImage()))
	if err != nil {
		return nil, err
	}

	if len(privateContent) == 0 && len(privateIndex) == 0 && baseSource == nil && imageSource == nil {
		return nil, nil
	}

	sources := &WorkingSetContentSources{
		Private: WorkingSetContentSource{
			Content: privateContent,
			Index:   privateIndex,
		},
	}
	if baseSource != nil {
		sources.BaseRootfs = *baseSource
	}
	if imageSource != nil {
		sources.Image = *imageSource
	}

	return sources, nil
}

func (mgr *SnapshotManager) getSharedWSSource(imageName string) (*WorkingSetContentSource, error) {
	if imageName == "" {
		if mgr.wsBaseCache != nil {
			return mgr.wsBaseCache, nil
		}
	} else {
		if cached, ok := mgr.wsImageCache.Load(imageName); ok {
			return cached.(*WorkingSetContentSource), nil
		}
	}

	chunkMap := make(map[string][]byte)

	// Legacy fallback path (aggregated content/index files)
	contentPath, indexPath := mgr.getSharedWSLocalPaths(imageName)
	contentObj, indexObj := mgr.getSharedWSObjectKeys(imageName)
	legacyContent, err := mgr.getOptionalSharedFile(contentPath, contentObj)
	if err != nil {
		return nil, err
	}
	legacyIndex, err := mgr.getOptionalSharedFile(indexPath, indexObj)
	if err != nil {
		return nil, err
	}
	if len(legacyContent) > 0 && len(legacyIndex) > 0 {
		if hashes, parseErr := parseWSHashIndex(legacyIndex); parseErr == nil {
			for i, hash := range hashes {
				start := i * 4096
				end := start + 4096
				if end > len(legacyContent) {
					break
				}
				chunkMap[hash] = append([]byte(nil), legacyContent[start:end]...)
			}
		}
	}

	// New path: merge per-hash chunk objects from local and remote
	localChunkDir := mgr.getSharedWSChunkLocalDir(imageName)
	if entries, dirErr := os.ReadDir(localChunkDir); dirErr == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			hash := entry.Name()
			if _, ok := chunkMap[hash]; ok {
				continue
			}
			data, readErr := os.ReadFile(filepath.Join(localChunkDir, hash))
			if readErr == nil && len(data) >= 4096 {
				chunkMap[hash] = append([]byte(nil), data[:4096]...)
			}
		}
	}

	if mgr.storage != nil {
		prefix := mgr.getSharedWSChunkObjectPrefix(imageName)
		objectKeys, listErr := mgr.storage.ListObjects(prefix, true)
		if listErr != nil {
			return nil, errors.Wrapf(listErr, "listing shared chunk objects with prefix %s", prefix)
		}

		for _, objectKey := range objectKeys {
			hash := filepath.Base(objectKey)
			if _, ok := chunkMap[hash]; ok {
				continue
			}

			data, dlErr := mgr.storage.DownloadObject(objectKey)
			if dlErr != nil || len(data) < 4096 {
				continue
			}
			chunkMap[hash] = append([]byte(nil), data[:4096]...)

			if mkErr := os.MkdirAll(localChunkDir, os.ModePerm); mkErr == nil {
				_ = os.WriteFile(filepath.Join(localChunkDir, hash), data[:4096], 0644)
			}
		}
	}

	if len(chunkMap) == 0 {
		return nil, nil
	}

	hashes := make([]string, 0, len(chunkMap))
	for hash := range chunkMap {
		hashes = append(hashes, hash)
	}
	sort.Strings(hashes)

	index, err := buildWSHashIndexCSV(hashes)
	if err != nil {
		return nil, err
	}

	content := make([]byte, 0, len(hashes)*4096)
	for _, hash := range hashes {
		content = append(content, chunkMap[hash]...)
	}

	source := &WorkingSetContentSource{Content: content, Index: index}
	if imageName == "" {
		mgr.wsBaseCache = source
	} else {
		mgr.wsImageCache.Store(imageName, source)
	}

	return source, nil
}

func (mgr *SnapshotManager) getOptionalSharedFile(localPath string, objectKey string) ([]byte, error) {
	if stat, err := os.Stat(localPath); err == nil && stat != nil {
		data, readErr := os.ReadFile(localPath)
		if readErr == nil {
			return data, nil
		}
	}

	if mgr.storage == nil {
		return nil, nil
	}

	exists, err := mgr.storage.Exists(objectKey)
	if err != nil {
		return nil, errors.Wrapf(err, "checking shared object %s", objectKey)
	}
	if !exists {
		return nil, nil
	}

	data, err := mgr.storage.DownloadObject(objectKey)
	if err != nil {
		return nil, errors.Wrapf(err, "downloading shared object %s", objectKey)
	}

	if err := os.MkdirAll(filepath.Dir(localPath), os.ModePerm); err == nil {
		_ = os.WriteFile(localPath, data, 0644)
	}

	return data, nil
}

func (mgr *SnapshotManager) GetWorkingSetPages(snap *Snapshot) ([]byte, error) {
	return mgr.GetSnapshotFileContent(snap, snap.GetWSFilePath())
}

func (mgr *SnapshotManager) GetUffdMemoryContent(snap *Snapshot, lazy bool) ([]byte, error) {
	if lazy {
		return mgr.GetSnapshotFileContent(snap, snap.GetRecipeFilePath())
	}
	return mgr.GetSnapshotFileContent(snap, snap.GetMemFilePath())
}

func (mgr *SnapshotManager) GetSnapshotFileContent(snap *Snapshot, localPath string) ([]byte, error) {
	return mgr.getFileContent(snap.GetId(), localPath)
}

func (mgr *SnapshotManager) getFileContent(revision, localPath string) ([]byte, error) {
	if stat, err := os.Stat(localPath); err == nil && stat != nil {
		data, err := os.ReadFile(localPath)
		if err == nil {
			return data, nil
		}
		log.Warnf("Failed to read local file %s: %v", localPath, err)
	}

	if mgr.storage == nil {
		return nil, errors.Errorf("file %s not found locally and remote storage is not configured", localPath)
	}

	objectKey := mgr.getObjectKey(revision, filepath.Base(localPath))
	data, err := mgr.storage.DownloadObject(objectKey)
	if err != nil {
		return nil, errors.Wrapf(err, "downloading object %s for fast download", objectKey)
	}

	if !mgr.cleanChunks {
		go func(path string, content []byte) {
			dir := filepath.Dir(path)
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				if err := os.MkdirAll(dir, os.ModePerm); err != nil {
					log.Warnf("Failed to create directory %s for background persist: %v", dir, err)
					return
				}
			}
			if err := os.WriteFile(path, content, 0644); err != nil {
				log.Warnf("Failed to write file %s in background: %v", path, err)
			}
		}(localPath, data)
	}

	return data, nil
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

func (mgr *SnapshotManager) IsChunkSensitive(hash [16]byte, image string) bool {
	return isHashSensitiveChunk(hash, image)
}

// EnsureRemoteSnapshotChunked converts an existing remote snapshot to chunked format in-place.
// If recipe_file already exists, hashes may be rewritten based on current security mode.
// If recipe_file is missing, the method derives recipe/chunks from mem_file.
func (mgr *SnapshotManager) EnsureRemoteSnapshotChunked(revision string) error {
	if !mgr.chunking {
		return errors.New("chunking must be enabled to convert remote snapshots")
	}
	if mgr.storage == nil {
		return errors.New("storage backend is not configured")
	}

	infoKey := mgr.getObjectKey(revision, "info_file")
	infoData, err := mgr.storage.DownloadObject(infoKey)
	if err != nil {
		return errors.Wrapf(err, "downloading info_file for %s", revision)
	}

	var tempSnap Snapshot
	if decodeErr := gob.NewDecoder(bytes.NewReader(infoData)).Decode(&tempSnap); decodeErr != nil {
		return errors.Wrapf(decodeErr, "decoding info_file for %s", revision)
	}

	recipeKey := mgr.getObjectKey(revision, "recipe_file")
	recipeData, err := mgr.storage.DownloadObject(recipeKey)
	if err != nil {
		return mgr.convertRemoteUnchunkedSnapshot(revision, tempSnap.Image)
	}

	return mgr.rewriteRemoteRecipeForSecurityMode(revision, tempSnap.Image, recipeData)
}

func (mgr *SnapshotManager) rewriteRemoteRecipeForSecurityMode(revision, image string, recipeData []byte) error {
	newRecipe := make([]byte, len(recipeData))
	copy(newRecipe, recipeData)

	modified := false
	for i := 0; i < len(recipeData); i += md5.Size {
		if i+md5.Size > len(recipeData) {
			break
		}

		var currentHash [md5.Size]byte
		copy(currentHash[:], recipeData[i:i+md5.Size])

		isSensitive := false
		if mgr.securityMode == "full" {
			isSensitive = true
		} else if mgr.securityMode == "partial" {
			isSensitive = mgr.IsChunkSensitive(currentHash, image)
		}

		if !isSensitive {
			continue
		}

		newHashBytes := md5.Sum(append(currentHash[:], []byte(revision)...))
		newHashStr := hex.EncodeToString(newHashBytes[:])

		newChunkKey := mgr.getObjectKey(chunkPrefix, newHashStr)
		exists, existsErr := mgr.storage.Exists(newChunkKey)
		if existsErr != nil {
			return errors.Wrapf(existsErr, "checking chunk existence for %s", newHashStr)
		}

		if !exists {
			currentHashStr := hex.EncodeToString(currentHash[:])
			oldChunkKey := mgr.getObjectKey(chunkPrefix, currentHashStr)
			data, dlErr := mgr.storage.DownloadObject(oldChunkKey)
			if dlErr != nil {
				return errors.Wrapf(dlErr, "downloading source chunk %s", oldChunkKey)
			}

			if upErr := mgr.storage.UploadObject(newChunkKey, bytes.NewReader(data), int64(len(data))); upErr != nil {
				return errors.Wrapf(upErr, "uploading rewritten chunk %s", newChunkKey)
			}
		}

		copy(newRecipe[i:i+md5.Size], newHashBytes[:])
		modified = true
	}

	if !modified {
		return nil
	}

	recipeKey := mgr.getObjectKey(revision, "recipe_file")
	if err := mgr.storage.UploadObject(recipeKey, bytes.NewReader(newRecipe), int64(len(newRecipe))); err != nil {
		return errors.Wrapf(err, "uploading updated recipe for %s", revision)
	}

	return nil
}

func (mgr *SnapshotManager) convertRemoteUnchunkedSnapshot(revision, image string) error {
	memKey := mgr.getObjectKey(revision, "mem_file")
	memData, err := mgr.storage.DownloadObject(memKey)
	if err != nil {
		return errors.Wrapf(err, "downloading mem_file for %s", revision)
	}

	recipe, _, err := mgr.uploadChunkedMemoryContent(bytes.NewReader(memData), revision, image)
	if err != nil {
		return errors.Wrapf(err, "uploading chunked content for %s", revision)
	}

	recipeKey := mgr.getObjectKey(revision, "recipe_file")
	if err := mgr.storage.UploadObject(recipeKey, bytes.NewReader(recipe), int64(len(recipe))); err != nil {
		return errors.Wrapf(err, "uploading recipe_file for %s", revision)
	}

	return nil
}
