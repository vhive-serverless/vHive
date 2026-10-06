package snapshotting

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestZstdWorkingSetRemoteRangeRoundTrip(t *testing.T) {
	store := newMemoryRangeStorage()
	base := t.TempDir()
	mgr := NewSnapshotManager(base, store, true, false, true, true, true, false,
		4096, 1024*1024, SecurityModePartial, 4, false, true)
	require.NoError(t, mgr.ConfigureCompression(CompressionConfig{
		WorkingSet: true,
		Codec:      CompressionCodecZstd,
		Level:      3,
		WSLayout:   "streams8-v1",
		Fetchers:   8,
	}))

	snap, err := mgr.InitSnapshot("zstd-ws-test", "test-image")
	require.NoError(t, err)
	raw := bytes.Repeat([]byte("private-working-set-page-content"), 16384)
	raw = raw[:len(raw)/4096*4096]
	rawPath := snap.GetWSPrivateContentFilePath()
	require.NoError(t, mgr.persistWorkingSetContent(snap.GetId(), rawPath, raw))

	_, rawUploaded := store.get(mgr.getObjectKey(snap.GetId(), filepath.Base(rawPath)))
	require.False(t, rawUploaded, "compressed mode must not publish a raw fallback")
	require.NoError(t, os.Remove(workingSetZstdPayloadPath(rawPath)))
	require.NoError(t, os.Remove(workingSetZstdManifestPath(rawPath)))

	decoded, release, err := mgr.getWorkingSetPathManaged(snap, rawPath)
	require.NoError(t, err)
	defer release()
	require.Equal(t, raw, decoded)
	require.Equal(t, 8, store.rangeOpenCount())
}

func TestZstdWorkingSetFailsClosedWithoutManifest(t *testing.T) {
	store := newMemoryRangeStorage()
	base := t.TempDir()
	mgr := NewSnapshotManager(base, store, true, false, true, true, true, false,
		4096, 1024*1024, SecurityModePartial, 2, false, true)
	require.NoError(t, mgr.ConfigureCompression(CompressionConfig{
		WorkingSet: true,
		Codec:      CompressionCodecZstd,
		Level:      3,
		WSLayout:   "streams8-v1",
		Fetchers:   8,
	}))
	snap, err := mgr.InitSnapshot("missing-manifest", "test-image")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(snap.GetWSPrivateContentFilePath(), []byte("raw must not be accepted"), 0644))

	_, release, err := mgr.getWorkingSetPathManaged(snap, snap.GetWSPrivateContentFilePath())
	release()
	require.ErrorContains(t, err, "manifest is required")
}

func TestZstdChunksUseIndependentNamespaceAndRoundTrip(t *testing.T) {
	store := newMemoryRangeStorage()
	raw := append(bytes.Repeat([]byte("compressible-page-"), 241), bytes.Repeat([]byte{0x5a}, 4096)...)
	raw = raw[:8192]

	uploader := NewSnapshotManager(t.TempDir(), store, true, false, true, false, false, false,
		4096, 1024*1024, SecurityModeFullDedup, 4, false, false)
	require.NoError(t, uploader.ConfigureCompression(CompressionConfig{
		Chunks:    true,
		Codec:     CompressionCodecZstd,
		Level:     3,
		FrameSize: 64 * 1024,
		Fetchers:  4,
	}))

	recipe, count, err := uploader.uploadChunkedMemoryContent(bytes.NewReader(raw), "chunk-zstd-test", "test-image")
	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.Len(t, recipe, 2*md5.Size)

	for index := 0; index < count; index++ {
		hash := hex.EncodeToString(recipe[index*md5.Size : (index+1)*md5.Size])
		_, rawExists := store.get(uploader.getObjectKey(chunkPrefix, hash))
		require.False(t, rawExists, "compressed chunks must not overwrite the historical raw namespace")
		stored, compressedExists := store.get(uploader.getObjectKey(uploader.activeChunkPrefix(), hash))
		require.True(t, compressedExists)
		require.NotEqual(t, raw[index*4096:(index+1)*4096], stored)

		local, err := uploader.DownloadAndReturnChunk(hash)
		require.NoError(t, err)
		require.Equal(t, raw[index*4096:(index+1)*4096], local)
	}

	receiver := NewSnapshotManager(t.TempDir(), store, true, false, true, false, false, false,
		4096, 1024*1024, SecurityModeFullDedup, 4, false, true)
	require.NoError(t, receiver.ConfigureCompression(CompressionConfig{
		Chunks:    true,
		Codec:     CompressionCodecZstd,
		Level:     3,
		FrameSize: 64 * 1024,
		Fetchers:  4,
	}))
	for index := 0; index < count; index++ {
		hash := hex.EncodeToString(recipe[index*md5.Size : (index+1)*md5.Size])
		remote, err := receiver.DownloadAndReturnChunk(hash)
		require.NoError(t, err)
		require.Equal(t, raw[index*4096:(index+1)*4096], remote)
	}
}

func TestZstdChunkCorruptionFailsClosed(t *testing.T) {
	store := newMemoryRangeStorage()
	raw := bytes.Repeat([]byte("zstd-corruption-check"), 200)
	raw = raw[:4096]
	mgr := NewSnapshotManager(t.TempDir(), store, true, false, true, false, false, false,
		4096, 1024*1024, SecurityModeFullDedup, 2, false, true)
	require.NoError(t, mgr.ConfigureCompression(CompressionConfig{
		Chunks: true, Codec: CompressionCodecZstd, Level: 3, FrameSize: 64 * 1024, Fetchers: 2,
	}))

	hashBytes := md5.Sum(raw)
	hash := hex.EncodeToString(hashBytes[:])
	stored, err := mgr.encodeChunkRepresentation(raw)
	require.NoError(t, err)
	stored[len(stored)/2] ^= 0xff
	store.put(mgr.getObjectKey(mgr.activeChunkPrefix(), hash), stored)

	_, err = mgr.DownloadAndReturnChunk(hash)
	require.ErrorContains(t, err, "decoding chunk")
}

func TestCompressionDisabledPreservesRawChunkLayout(t *testing.T) {
	store := newMemoryRangeStorage()
	raw := bytes.Repeat([]byte{0x42}, 4096)
	mgr := NewSnapshotManager(t.TempDir(), store, true, false, true, false, false, false,
		4096, 1024*1024, SecurityModeFullDedup, 2, false, true)
	require.NoError(t, mgr.ConfigureCompression(DefaultCompressionConfig()))

	recipe, count, err := mgr.uploadChunkedMemoryContent(bytes.NewReader(raw), "raw-layout-test", "test-image")
	require.NoError(t, err)
	require.Equal(t, 1, count)
	hash := hex.EncodeToString(recipe)
	stored, ok := store.get(mgr.getObjectKey(chunkPrefix, hash))
	require.True(t, ok)
	require.Equal(t, raw, stored)
	_, compressed := store.get(mgr.getObjectKey(chunkZstdPrefix+"_l3", hash))
	require.False(t, compressed)
}

func TestZstdChunkUploadManyErrorsDoesNotDeadlock(t *testing.T) {
	store := &failingExistsStorage{memoryRangeStorage: newMemoryRangeStorage()}
	mgr := NewSnapshotManager(t.TempDir(), store, true, false, true, false, false, false,
		4096, 1024*1024, SecurityModeFullDedup, 4, false, true)
	require.NoError(t, mgr.ConfigureCompression(CompressionConfig{
		Chunks: true, Codec: CompressionCodecZstd, Level: 3, FrameSize: 64 * 1024, Fetchers: 4,
	}))

	raw := make([]byte, 256*4096)
	for page := 0; page < 256; page++ {
		binary.LittleEndian.PutUint64(raw[page*4096:], uint64(page))
	}
	_, count, err := mgr.uploadChunkedMemoryContent(bytes.NewReader(raw), "many-errors", "test-image")
	require.Equal(t, 256, count)
	require.ErrorContains(t, err, "forced Exists failure")
}

func TestConverterCreatesCompressedObjectsWithoutChangingRawRecipe(t *testing.T) {
	store := newMemoryRangeStorage()
	raw := bytes.Repeat([]byte("converter-zstd-source"), 200)
	raw = raw[:4096]
	hashBytes := md5.Sum(raw)
	hash := hex.EncodeToString(hashBytes[:])
	store.put(chunkPrefix+"/"+hash[:2]+"/"+hash, raw)

	mgr := NewSnapshotManager(t.TempDir(), store, true, false, true, false, false, false,
		4096, 1024*1024, SecurityModeFullDedup, 2, false, true)
	require.NoError(t, mgr.ConfigureCompression(CompressionConfig{
		Chunks: true, Codec: CompressionCodecZstd, Level: 3, FrameSize: 64 * 1024, Fetchers: 2,
	}))
	require.NoError(t, mgr.rewriteRemoteRecipeForSecurityMode("converter-zstd-test", "test-image", hashBytes[:]))

	stored, ok := store.get(mgr.getObjectKey(mgr.activeChunkPrefix(), hash))
	require.True(t, ok)
	decoded, err := mgr.decodeChunkRepresentation(stored)
	require.NoError(t, err)
	require.Equal(t, raw, decoded)
	_, recipeUploaded := store.get(mgr.getObjectKey("converter-zstd-test", "recipe_file"))
	require.False(t, recipeUploaded, "physical recompression must not rewrite an unchanged provenance recipe")
}

func TestPreservedRecipeChunkConversionDoesNotResaltPartialKeys(t *testing.T) {
	store := newMemoryRangeStorage()
	raw := bytes.Repeat([]byte("private-partial-page"), 256)
	raw = raw[:4096]
	rawHash := md5.Sum(raw)
	revision := "partial-revision"
	saltedHash := md5.Sum(append(rawHash[:], []byte(revision)...))
	saltedHashString := hex.EncodeToString(saltedHash[:])
	store.put(chunkPrefix+"/"+saltedHashString[:2]+"/"+saltedHashString, raw)
	store.put(revision+"/recipe_file", saltedHash[:])

	mgr := NewSnapshotManager(t.TempDir(), store, true, false, true, false, false, false,
		4096, 1024*1024, SecurityModePartial, 2, false, true)
	require.NoError(t, mgr.ConfigureCompression(CompressionConfig{
		Chunks: true, Codec: CompressionCodecZstd, Level: 3, FrameSize: 64 * 1024, Fetchers: 2,
	}))
	require.NoError(t, mgr.EnsureRemoteSnapshotChunkRepresentation(revision))

	recipe, ok := store.get(revision + "/recipe_file")
	require.True(t, ok)
	require.Equal(t, saltedHash[:], recipe, "matched codec conversion must preserve provenance-derived keys")
	stored, ok := store.get(mgr.getObjectKey(mgr.activeChunkPrefix(), saltedHashString))
	require.True(t, ok)
	decoded, err := mgr.decodeChunkRepresentation(stored)
	require.NoError(t, err)
	require.Equal(t, raw, decoded)

	doubleSalted := md5.Sum(append(saltedHash[:], []byte(revision)...))
	doubleSaltedString := hex.EncodeToString(doubleSalted[:])
	_, exists := store.get(mgr.getObjectKey(mgr.activeChunkPrefix(), doubleSaltedString))
	require.False(t, exists, "private key must not be salted a second time")
}

func TestPreservedRecipeChunkConversionRejectsMalformedRecipe(t *testing.T) {
	store := newMemoryRangeStorage()
	store.put("bad-revision/recipe_file", []byte("not-a-whole-md5"))
	mgr := NewSnapshotManager(t.TempDir(), store, true, false, true, false, false, false,
		4096, 1024*1024, SecurityModePartial, 2, false, true)
	require.NoError(t, mgr.ConfigureCompression(CompressionConfig{
		Chunks: true, Codec: CompressionCodecZstd, Level: 3, FrameSize: 64 * 1024, Fetchers: 2,
	}))
	require.ErrorContains(t, mgr.EnsureRemoteSnapshotChunkRepresentation("bad-revision"), "invalid length")
}

type memoryRangeStorage struct {
	mu         sync.Mutex
	objects    map[string][]byte
	rangeOpens int
}

type failingExistsStorage struct {
	*memoryRangeStorage
}

func (store *failingExistsStorage) Exists(string) (bool, error) {
	return false, fmt.Errorf("forced Exists failure")
}

func newMemoryRangeStorage() *memoryRangeStorage {
	return &memoryRangeStorage{objects: make(map[string][]byte)}
}

func (store *memoryRangeStorage) UploadObject(key string, reader io.Reader, _ int64) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	store.mu.Lock()
	store.objects[key] = append([]byte(nil), data...)
	store.mu.Unlock()
	return nil
}

func (store *memoryRangeStorage) DownloadObject(key string) ([]byte, error) {
	data, ok := store.get(key)
	if !ok {
		return nil, fmt.Errorf("missing object %s", key)
	}
	return data, nil
}

func (store *memoryRangeStorage) OpenObject(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := store.DownloadObject(key)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (store *memoryRangeStorage) Exists(key string) (bool, error) {
	_, ok := store.get(key)
	return ok, nil
}

func (store *memoryRangeStorage) ListObjects(prefix string, _ bool) ([]string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	keys := make([]string, 0)
	for key := range store.objects {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

func (store *memoryRangeStorage) UploadFile(key, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	store.mu.Lock()
	store.objects[key] = append([]byte(nil), data...)
	store.mu.Unlock()
	return nil
}

func (store *memoryRangeStorage) DownloadFile(key, path string) error {
	data, err := store.DownloadObject(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (store *memoryRangeStorage) OpenObjectRange(_ context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	data, ok := store.get(key)
	if !ok {
		return nil, fmt.Errorf("missing object %s", key)
	}
	if offset < 0 || length <= 0 || offset+length > int64(len(data)) {
		return nil, fmt.Errorf("invalid range offset=%d length=%d size=%d", offset, length, len(data))
	}
	store.mu.Lock()
	store.rangeOpens++
	store.mu.Unlock()
	return io.NopCloser(&shortReader{reader: bytes.NewReader(data[offset : offset+length]), max: 97}), nil
}

func (store *memoryRangeStorage) get(key string) ([]byte, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	data, ok := store.objects[key]
	return append([]byte(nil), data...), ok
}

func (store *memoryRangeStorage) put(key string, data []byte) {
	store.mu.Lock()
	store.objects[key] = append([]byte(nil), data...)
	store.mu.Unlock()
}

func (store *memoryRangeStorage) rangeOpenCount() int {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.rangeOpens
}

type shortReader struct {
	reader *bytes.Reader
	max    int
}

func (reader *shortReader) Read(data []byte) (int, error) {
	if len(data) > reader.max {
		data = data[:reader.max]
	}
	return reader.reader.Read(data)
}
