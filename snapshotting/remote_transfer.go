package snapshotting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const remoteDescriptorArtifact = ".snapshot-descriptor.json"

// remoteSnapshotTransfer keeps the whole-file remote protocol separate from
// SnapshotManager's local lifecycle. Chunking deliberately belongs to stage 4.
type remoteSnapshotTransfer struct {
	store       ArtifactStore
	cacheSnaps  bool
	chunkSize   int
	chunkCache  ChunkCache
	workingSets *WorkingSetRepository
	// reconstructMemory is intentionally independent from chunking. Chunked
	// snapshots can instead be consumed through their recipe by a page server.
	reconstructMemory bool

	mu        sync.Mutex
	downloads map[string]*remoteDownload
}

type remoteDownload struct {
	done chan struct{}
	desc *SnapshotDescriptor
	err  error
}

type memoryRecipeCatalog interface {
	SetMemoryRecipe(revision, recipe string) error
}

func persistMemoryRecipe(catalog Catalog, revision, recipe string) error {
	if recipe == "" {
		return nil
	}
	if local, ok := catalog.(memoryRecipeCatalog); ok {
		return local.SetMemoryRecipe(revision, recipe)
	}
	return nil
}

func newRemoteSnapshotTransfer(store ArtifactStore, cacheSnaps bool) *remoteSnapshotTransfer {
	return &remoteSnapshotTransfer{
		store:             store,
		cacheSnaps:        cacheSnaps,
		reconstructMemory: false,
		downloads:         make(map[string]*remoteDownload),
	}
}

func (r *remoteSnapshotTransfer) enableChunkedMemory(chunkSize int) error {
	if chunkSize <= 0 {
		return fmt.Errorf("chunk size must be positive")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chunkSize = chunkSize
	return nil
}

func (r *remoteSnapshotTransfer) setChunkCache(cache ChunkCache) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chunkCache = cache
}

func (r *remoteSnapshotTransfer) setMemoryReconstruction(enabled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reconstructMemory = enabled
}

func (r *remoteSnapshotTransfer) setWorkingSetRepository(repository *WorkingSetRepository) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workingSets = repository
}

func (r *remoteSnapshotTransfer) hasDownload(revision string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.downloads[revision] != nil
}

func (r *remoteSnapshotTransfer) publish(ctx context.Context, catalog Catalog, baseFolder, revision string) error {
	desc, err := catalog.Get(revision)
	if err != nil {
		return err
	}
	if err := validateRemoteDescriptor(desc, revision); err != nil {
		return err
	}

	// Upload payloads before the descriptor. The descriptor is the remote
	// readiness marker and therefore must be last.
	r.mu.Lock()
	chunkSize := r.chunkSize
	workingSets := r.workingSets
	r.mu.Unlock()
	artifacts := []string{desc.Artifacts.VMState, desc.Artifacts.Info, desc.Artifacts.Patch}
	if chunkSize == 0 {
		artifacts = append([]string{desc.Artifacts.Memory}, artifacts...)
	}
	tracePath := filepath.Join(baseFolder, revision, desc.Artifacts.WorkingSetTrace)
	if _, err := os.Stat(tracePath); err == nil {
		artifacts = append(artifacts, desc.Artifacts.WorkingSetTrace)
		copy := *desc
		desc = &copy
		desc.WorkingSetTrace = true
		pagesPath := filepath.Join(baseFolder, revision, desc.Artifacts.WorkingSetPages)
		if _, err := os.Stat(pagesPath); err == nil {
			artifacts = append(artifacts, desc.Artifacts.WorkingSetPages)
			desc.WorkingSet = true
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat snapshot working-set artifact %s: %w", desc.Artifacts.WorkingSetPages, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat snapshot working-set artifact %s: %w", desc.Artifacts.WorkingSetTrace, err)
	}
	for _, artifact := range artifacts {
		file := filepath.Join(baseFolder, revision, artifact)
		if artifact == desc.Artifacts.Patch {
			if _, statErr := os.Stat(file); os.IsNotExist(statErr) {
				continue
			} else if statErr != nil {
				return fmt.Errorf("stat snapshot artifact %s: %w", artifact, statErr)
			}
		}
		if err := putFile(ctx, r.store, revision, artifact, file); err != nil {
			return err
		}
	}
	if workingSets != nil && desc.WorkingSetTrace && desc.WorkingSet {
		if _, err := workingSets.PublishFiles(ctx, revision, desc.Image, tracePath, filepath.Join(baseFolder, revision, desc.Artifacts.WorkingSetPages)); err != nil {
			return fmt.Errorf("publish provenance working set: %w", err)
		}
		copy := *desc
		desc = &copy
		desc.ProvenanceWorkingSet = true
	}
	if chunkSize > 0 {
		recipe, err := uploadChunkedMemory(ctx, r.store, filepath.Join(baseFolder, revision, desc.Artifacts.Memory), chunkSize)
		if err != nil {
			return err
		}
		if err := putRecipe(ctx, r.store, revision, recipe); err != nil {
			return err
		}
		copy := *desc
		desc = &copy
		desc.MemoryRecipe = memoryRecipeArtifact
		if err := persistMemoryRecipe(catalog, revision, desc.MemoryRecipe); err != nil {
			return err
		}
	}
	data, err := json.Marshal(desc)
	if err != nil {
		return fmt.Errorf("encode remote descriptor: %w", err)
	}
	key, err := RevisionArtifactKey(revision, remoteDescriptorArtifact)
	if err != nil {
		return err
	}
	if err := r.store.Put(ctx, key, bytes.NewReader(data), int64(len(data))); err != nil {
		return fmt.Errorf("upload remote descriptor for %s: %w", revision, err)
	}
	return nil
}

// publishWorkingSet updates a remote descriptor with a recorded trace and,
// when available, its coalesced page bytes. It deliberately does not touch
// memory, VM state, or other snapshot artifacts.
func (r *remoteSnapshotTransfer) publishWorkingSet(ctx context.Context, catalog Catalog, baseFolder, revision string) (bool, error) {
	desc, err := catalog.Get(revision)
	if err != nil {
		return false, err
	}
	if err := validateRemoteDescriptor(desc, revision); err != nil {
		return false, err
	}
	tracePath := filepath.Join(baseFolder, revision, desc.Artifacts.WorkingSetTrace)
	if _, err := os.Stat(tracePath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat snapshot working-set artifact %s: %w", desc.Artifacts.WorkingSetTrace, err)
	}
	for _, artifact := range []string{desc.Artifacts.WorkingSetTrace} {
		if err := putFile(ctx, r.store, revision, artifact, filepath.Join(baseFolder, revision, artifact)); err != nil {
			return false, err
		}
	}
	copy := *desc
	copy.WorkingSetTrace = true
	r.mu.Lock()
	workingSets := r.workingSets
	r.mu.Unlock()
	pagesPath := filepath.Join(baseFolder, revision, desc.Artifacts.WorkingSetPages)
	if _, err := os.Stat(pagesPath); err == nil {
		if workingSets != nil {
			if _, err := workingSets.PublishFiles(ctx, revision, desc.Image, tracePath, pagesPath); err != nil {
				return false, fmt.Errorf("publish provenance working set: %w", err)
			}
			copy.ProvenanceWorkingSet = true
		}
		if err := putFile(ctx, r.store, revision, desc.Artifacts.WorkingSetPages, pagesPath); err != nil {
			return false, err
		}
		copy.WorkingSet = true
	} else if os.IsNotExist(err) {
		copy.WorkingSet = false
		copy.ProvenanceWorkingSet = false
	} else {
		return false, fmt.Errorf("stat snapshot working-set artifact %s: %w", desc.Artifacts.WorkingSetPages, err)
	}
	data, err := json.Marshal(&copy)
	if err != nil {
		return false, fmt.Errorf("encode remote descriptor: %w", err)
	}
	key, err := RevisionArtifactKey(revision, remoteDescriptorArtifact)
	if err != nil {
		return false, err
	}
	if err := r.store.Put(ctx, key, bytes.NewReader(data), int64(len(data))); err != nil {
		return false, fmt.Errorf("upload remote descriptor for %s: %w", revision, err)
	}
	return true, nil
}

func (r *remoteSnapshotTransfer) download(ctx context.Context, catalog Catalog, baseFolder, revision string) (*SnapshotDescriptor, error) {
	r.mu.Lock()
	if active := r.downloads[revision]; active != nil {
		r.mu.Unlock()
		select {
		case <-active.done:
			return active.desc, active.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	active := &remoteDownload{done: make(chan struct{})}
	r.downloads[revision] = active
	r.mu.Unlock()

	active.desc, active.err = r.downloadOnce(ctx, catalog, baseFolder, revision)
	close(active.done)
	r.mu.Lock()
	delete(r.downloads, revision)
	r.mu.Unlock()
	return active.desc, active.err
}

func (r *remoteSnapshotTransfer) downloadOnce(ctx context.Context, catalog Catalog, baseFolder, revision string) (_ *SnapshotDescriptor, retErr error) {
	// A previous downloader may have committed between the cache-miss lookup
	// and this caller becoming the transfer leader.
	if local, err := catalog.Get(revision); err == nil {
		return local, nil
	} else if !errors.Is(err, ErrSnapshotNotFound) && !errors.Is(err, ErrSnapshotNotReady) {
		return nil, err
	}
	desc, err := r.getDescriptor(ctx, revision)
	if err != nil {
		return nil, err
	}
	if err := validateRemoteDescriptor(desc, revision); err != nil {
		return nil, err
	}
	if _, err := catalog.Begin(revision, desc.Image); err != nil {
		return nil, err
	}
	if err := persistMemoryRecipe(catalog, revision, desc.MemoryRecipe); err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			_ = catalog.Delete(revision)
		}
	}()

	artifacts := []string{desc.Artifacts.VMState, desc.Artifacts.Info}
	if desc.WorkingSetTrace {
		artifacts = append(artifacts, desc.Artifacts.WorkingSetTrace)
	}
	if desc.WorkingSet {
		artifacts = append(artifacts, desc.Artifacts.WorkingSetPages)
	}
	// The manifest and private source are revision scoped. Shared sources stay
	// immutable in the shared namespace and are read directly by the lazy page
	// source, so they must not be copied into a local snapshot directory.
	if desc.ProvenanceWorkingSet {
		for _, artifact := range []string{provenanceManifestArtifact, privateContentArtifact, privateIndexArtifact} {
			if err := getFile(ctx, r.store, revision, artifact, filepath.Join(baseFolder, revision, artifact)); err != nil {
				return nil, err
			}
		}
	}
	for _, artifact := range artifacts {
		if err := getFile(ctx, r.store, revision, artifact, filepath.Join(baseFolder, revision, artifact)); err != nil {
			return nil, err
		}
	}
	if desc.MemoryRecipe == "" {
		if err := getFile(ctx, r.store, revision, desc.Artifacts.Memory, filepath.Join(baseFolder, revision, desc.Artifacts.Memory)); err != nil {
			return nil, err
		}
	} else {
		r.mu.Lock()
		reconstructMemory := r.reconstructMemory
		r.mu.Unlock()
		if reconstructMemory {
			recipe, err := getRecipe(ctx, r.store, revision)
			if err != nil {
				return nil, err
			}
			memoryPath := filepath.Join(baseFolder, revision, desc.Artifacts.Memory)
			temporary, err := os.CreateTemp(filepath.Dir(memoryPath), ".reconstructed-memory-*")
			if err != nil {
				return nil, fmt.Errorf("create reconstructed memory file: %w", err)
			}
			temporaryName := temporary.Name()
			defer os.Remove(temporaryName)
			r.mu.Lock()
			cache := r.chunkCache
			r.mu.Unlock()
			// With no persistent cache configured, keep chunks only for this
			// reconstruction. A recipe may reference one content-addressed chunk
			// more than once, and re-downloading it during a single restore is
			// unnecessary.
			if cache == nil {
				cache = newMemoryChunkCache()
			}
			if err := ReconstructMemoryWithCache(ctx, r.store, cache, recipe, temporary); err != nil {
				temporary.Close()
				return nil, err
			}
			if err := temporary.Close(); err != nil {
				return nil, fmt.Errorf("close reconstructed memory file: %w", err)
			}
			if err := os.Rename(temporaryName, memoryPath); err != nil {
				return nil, fmt.Errorf("publish reconstructed memory file: %w", err)
			}
		}
	}
	// A patch is needed only by devmapper snapshots, so its absence is valid.
	patchPath := filepath.Join(baseFolder, revision, desc.Artifacts.Patch)
	if err := getFile(ctx, r.store, revision, desc.Artifacts.Patch, patchPath); err != nil {
		if !isArtifactNotFound(err) {
			return nil, err
		}
	}
	if err := validateSnapshotInfo(filepath.Join(baseFolder, revision, desc.Artifacts.Info), desc); err != nil {
		return nil, err
	}
	if err := catalog.Commit(revision); err != nil {
		return nil, err
	}
	return desc, nil
}

func (r *remoteSnapshotTransfer) getDescriptor(ctx context.Context, revision string) (*SnapshotDescriptor, error) {
	key, err := RevisionArtifactKey(revision, remoteDescriptorArtifact)
	if err != nil {
		return nil, err
	}
	reader, err := r.store.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("download remote descriptor for %s: %w", revision, err)
	}
	defer reader.Close()
	var desc SnapshotDescriptor
	if err := json.NewDecoder(reader).Decode(&desc); err != nil {
		return nil, fmt.Errorf("decode remote descriptor for %s: %w", revision, err)
	}
	return &desc, nil
}

func validateRemoteDescriptor(desc *SnapshotDescriptor, revision string) error {
	// Image-less base snapshots are valid: they are restored before a function
	// image is selected and pulled. Function snapshots still carry their image
	// identity when one exists.
	if desc == nil || desc.Revision != revision || !desc.Ready {
		return fmt.Errorf("invalid remote descriptor for %s", revision)
	}
	for _, artifact := range []string{desc.Artifacts.VMState, desc.Artifacts.Memory, desc.Artifacts.Info, desc.Artifacts.Patch} {
		if _, err := RevisionArtifactKey(revision, artifact); err != nil {
			return fmt.Errorf("invalid remote descriptor: %w", err)
		}
	}
	if desc.MemoryRecipe != "" && desc.MemoryRecipe != memoryRecipeArtifact {
		return fmt.Errorf("invalid remote memory recipe %q", desc.MemoryRecipe)
	}
	return nil
}

func putFile(ctx context.Context, store ArtifactStore, revision, artifact, filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("open snapshot artifact %s: %w", artifact, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat snapshot artifact %s: %w", artifact, err)
	}
	key, err := RevisionArtifactKey(revision, artifact)
	if err != nil {
		return err
	}
	if err := store.Put(ctx, key, file, info.Size()); err != nil {
		return fmt.Errorf("upload snapshot artifact %s: %w", artifact, err)
	}
	return nil
}

func getFile(ctx context.Context, store ArtifactStore, revision, artifact, filename string) error {
	key, err := RevisionArtifactKey(revision, artifact)
	if err != nil {
		return err
	}
	reader, err := store.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("download snapshot artifact %s: %w", artifact, err)
	}
	defer reader.Close()
	temporary, err := os.CreateTemp(filepath.Dir(filename), ".remote-artifact-*")
	if err != nil {
		return fmt.Errorf("create local snapshot artifact %s: %w", artifact, err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := io.Copy(temporary, reader); err != nil {
		temporary.Close()
		return fmt.Errorf("copy snapshot artifact %s: %w", artifact, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close snapshot artifact %s: %w", artifact, err)
	}
	if err := os.Rename(temporaryName, filename); err != nil {
		return fmt.Errorf("publish local snapshot artifact %s: %w", artifact, err)
	}
	return nil
}

func validateSnapshotInfo(filename string, desc *SnapshotDescriptor) error {
	snapshot := NewSnapshot(desc.Revision, filepath.Dir(filepath.Dir(filename)), desc.Image)
	if err := snapshot.LoadSnapInfo(filename); err != nil {
		return fmt.Errorf("validate snapshot metadata: %w", err)
	}
	if snapshot.GetImage() != desc.Image {
		return fmt.Errorf("snapshot metadata image mismatch: got %q, want %q", snapshot.GetImage(), desc.Image)
	}
	return nil
}

func isArtifactNotFound(err error) bool {
	return os.IsNotExist(err) || errors.Is(err, ErrArtifactNotFound)
}
