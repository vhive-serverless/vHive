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
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkg/errors"

	log "github.com/sirupsen/logrus"

	"github.com/vhive-serverless/vhive/storage"
)

const (
	chunkPrefix = "_chunks"
	chunkSize   = 512 * 1024 // 512 KB
)

func GetChunkSize() uint64 {
	return chunkSize
}

// SnapshotManager manages snapshots stored on the node.
type SnapshotManager struct {
	sync.Mutex
	// Stored snapshots (identified by the function instance revision, which is provided by the `K_REVISION` environment
	// variable of knative).
	snapshots  map[string]*Snapshot
	baseFolder string
	chunking   bool
	lazy       bool
	wsPulling  bool

	// Used to store remote snapshots
	storage storage.ObjectStorage
}

func NewSnapshotManager(baseFolder string, store storage.ObjectStorage, chunking, skipCleanup, lazy, wsPulling bool) *SnapshotManager {
	manager := &SnapshotManager{
		snapshots:  make(map[string]*Snapshot),
		baseFolder: baseFolder,
		chunking:   chunking,
		storage:    store,
		wsPulling:  wsPulling,
		lazy:       lazy,
	}

	// Clean & init basefolder unless skipping is requested
	if !skipCleanup {
		_ = os.RemoveAll(manager.baseFolder)
	}
	_ = os.MkdirAll(manager.baseFolder, os.ModePerm)
	if chunking {
		_ = os.MkdirAll(filepath.Join(manager.baseFolder, chunkPrefix), os.ModePerm)
	}

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

	// Return snapshot for supplied revision
	return mgr.snapshots[revision], nil
}

// InitSnapshot initializes a snapshot by adding its metadata to the SnapshotManager. Once the snapshot has
// been created, CommitSnapshot must be run to finalize the snapshot creation and make the snapshot available for use.
func (mgr *SnapshotManager) InitSnapshot(revision, image string) (*Snapshot, error) {
	mgr.Lock()

	logger := log.WithFields(log.Fields{"revision": revision, "image": image})
	logger.Debug("Initializing snapshot corresponding to revision and image")

	if _, present := mgr.snapshots[revision]; present {
		mgr.Unlock()
		return nil, errors.New(fmt.Sprintf("Add: Snapshot for revision %s already exists", revision))
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
	mgr.Lock()
	defer mgr.Unlock()

	if !mgr.chunking {
		return nil
	}
	os.RemoveAll(filepath.Join(mgr.baseFolder, chunkPrefix))
	os.MkdirAll(filepath.Join(mgr.baseFolder, chunkPrefix), os.ModePerm)
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

func (mgr *SnapshotManager) uploadMemFile(snap *Snapshot) error {
	if !mgr.chunking {
		return mgr.uploadFile(snap.GetId(), snap.GetMemFilePath())
	}

	file, err := os.Open(snap.GetMemFilePath())
	if err != nil {
		return errors.Wrapf(err, "opening memory file for chunked upload")
	}
	defer file.Close()

	buffer := make([]byte, chunkSize)
	chunkIndex := 0
	recipe := make([]byte, 0)
	for {
		n, err := io.ReadFull(file, buffer)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return errors.Wrapf(err, "reading chunk %d from memory file", chunkIndex)
		}
		if n == 0 {
			break
		}

		// Compute MD5 hash of chunk
		hash := md5.Sum(buffer[:n])
		recipe = append(recipe, hash[:]...)
		chunkHash := hex.EncodeToString(hash[:])
		chunkFilePath := filepath.Join(mgr.baseFolder, chunkPrefix, chunkHash)

		if _, err := os.Stat(chunkFilePath); err == nil {
			// Chunk file already exists, skip uploading
			chunkIndex++
			continue
		}

		chunkFile, err := os.Create(chunkFilePath)
		if err != nil {
			return errors.Wrapf(err, "creating chunk file %s", chunkFilePath)
		}

		if _, err := chunkFile.Write(buffer); err != nil {
			chunkFile.Close()
			return errors.Wrapf(err, "writing to chunk file %s", chunkFilePath)
		}
		mgr.uploadFile(chunkPrefix, chunkFilePath)

		chunkFile.Close()
		// os.Remove(chunkFilePath)
		chunkIndex++
		if err == io.EOF {
			break
		}
	}

	// Upload recipe file
	recipeFilePath := snap.GetRecipeFilePath()
	recipeFile, err := os.Create(recipeFilePath)
	if err != nil {
		return errors.Wrapf(err, "creating recipe file for chunked upload")
	}
	defer recipeFile.Close()

	if _, err := recipeFile.Write(recipe); err != nil {
		return errors.Wrapf(err, "writing recipe file for chunked upload")
	}

	mgr.uploadFile(snap.GetId(), recipeFilePath)
	os.Remove(recipeFilePath)

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

	// stat, _ := os.Stat(snap.GetMemFilePath())
	// log.Infof("Downloaded memory file for snapshot %s, size is %d", snap.GetId(), stat.Size())

	if err := mgr.CommitSnapshot(revision); err != nil {
		return nil, errors.Wrap(err, "committing snapshot")
	}

	return snap, nil
}

func (mgr *SnapshotManager) downloadMemFile(snap *Snapshot) error {
	if !mgr.chunking {
		return mgr.downloadFile(snap.GetId(), snap.GetMemFilePath(), filepath.Base(snap.GetMemFilePath()))
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

	recipeFile, err := os.Open(recipeFilePath)
	if err != nil {
		return errors.Wrapf(err, "opening recipe file for chunked download")
	}
	defer recipeFile.Close()

	recipe, err := io.ReadAll(recipeFile)
	if err != nil {
		return errors.Wrapf(err, "reading recipe file for chunked download")
	}

	chunkIndex := 0
	for hashStart := 0; hashStart < len(recipe); hashStart += md5.Size {
		hashEnd := hashStart + md5.Size
		if hashEnd > len(recipe) {
			break
		}
		hash := hex.EncodeToString(recipe[hashStart:hashEnd])

		chunkFilePath := filepath.Join(mgr.baseFolder, chunkPrefix, hash)
		if err := mgr.DownloadChunk(hash); err != nil {
			return errors.Wrapf(err, "downloading chunk %d of memory file", chunkIndex)
		}

		chunkFile, err := os.Open(chunkFilePath)
		if err != nil {
			return errors.Wrapf(err, "opening chunk file %s", chunkFilePath)
		}

		if _, err := io.Copy(outFile, chunkFile); err != nil {
			chunkFile.Close()
			return errors.Wrapf(err, "writing chunk %d to memory file", chunkIndex)
		}

		chunkFile.Close()
		chunkIndex++
	}

	return nil
}

func (mgr *SnapshotManager) DownloadChunk(hash string) error {
	chunkFilePath := filepath.Join(mgr.baseFolder, chunkPrefix, hash)

	if _, err := os.Stat(chunkFilePath); err == nil { // Chunk file exists locally, use it
		return nil
	}

	// Chunk file does not exist locally, download it
	return mgr.downloadFile(chunkPrefix, chunkFilePath, hash)
}

func (mgr *SnapshotManager) GetChunkFilePath(hash string) string {
	return filepath.Join(mgr.baseFolder, chunkPrefix, hash)
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

func (mgr *SnapshotManager) downloadWorkingSet(snap *Snapshot) error {
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
		chunkIndex := byteOffset / chunkSize

		// Get chunk hash from recipe
		hashStart := int(chunkIndex) * md5.Size
		hashEnd := hashStart + md5.Size
		if hashEnd > len(recipe) {
			continue // Page is beyond recipe bounds
		}

		hash := hex.EncodeToString(recipe[hashStart:hashEnd])
		chunksToLoad[hash] = true
	}

	// Download only the working set chunks
	for hash := range chunksToLoad {
		if err := mgr.DownloadChunk(hash); err != nil {
			return errors.Wrapf(err, "downloading working set chunk %s", hash)
		}
	}

	log.Infof("Finished downloading working set for snapshot %s, %d chunks downloaded", snap.GetId(), len(chunksToLoad))

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
