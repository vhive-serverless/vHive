package snapshotting

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vhive-serverless/vhive/memory/manager"
)

type provenanceFallback struct{ data []byte }

func (s *provenanceFallback) ReadAt(_ context.Context, offset, length uint64) (manager.PageData, error) {
	if offset > uint64(len(s.data)) || length > uint64(len(s.data))-offset {
		return manager.PageData{}, errors.New("outside fallback")
	}
	return manager.PageData{Bytes: append([]byte(nil), s.data[offset:offset+length]...)}, nil
}
func (*provenanceFallback) Close() error { return nil }

func TestProvenanceWorkingSetDoesNotShareEqualPrivateBytes(t *testing.T) {
	store := NewMemoryArtifactStore()
	repository, err := NewWorkingSetRepository(store, AllPrivateProvenance{}, "base-v1")
	require.NoError(t, err)
	_, err = repository.Publish(context.Background(), "revision-a", "registry.test/image:v1", 4, []WorkingSetPage{{PFN: 0, Bytes: []byte("same")}})
	require.NoError(t, err)
	_, err = repository.Publish(context.Background(), "revision-b", "registry.test/image:v1", 4, []WorkingSetPage{{PFN: 0, Bytes: []byte("same")}})
	require.NoError(t, err)
	shared, err := store.List(context.Background(), "shared/")
	require.NoError(t, err)
	require.Empty(t, shared, "equal bytes do not imply shared provenance")
	for _, revision := range []string{"revision-a", "revision-b"} {
		key, _ := RevisionArtifactKey(revision, privateContentArtifact)
		_, err := store.Stat(context.Background(), key)
		require.NoError(t, err)
	}
}

func TestProvenanceWorkingSetServesPrivateBaseAndImageSources(t *testing.T) {
	store := NewMemoryArtifactStore()
	policy := FixtureProvenance{4: BaseRootfsScope, 8: ImageScope}
	repository, err := NewWorkingSetRepository(store, policy, "base/rootfs@sha256:abc")
	require.NoError(t, err)
	pages := []WorkingSetPage{{PFN: 0, Bytes: []byte("priv")}, {PFN: 4, Bytes: []byte("base")}, {PFN: 8, Bytes: []byte("imag")}}
	manifest, err := repository.Publish(context.Background(), "revision-a", "registry.test/team/image:v1", 4, pages)
	require.NoError(t, err)
	require.NotEmpty(t, manifest.Private.Content)
	require.NotEmpty(t, manifest.Base.Content)
	require.NotEmpty(t, manifest.Image.Content)

	source, err := NewProvenanceWorkingSetPageSource(context.Background(), store, "revision-a", &provenanceFallback{data: []byte("fallback-data")})
	require.NoError(t, err)
	for _, page := range pages {
		got, err := source.ReadAt(context.Background(), page.PFN, 4)
		require.NoError(t, err)
		require.Equal(t, page.Bytes, got.Bytes)
	}
}

func TestPublishFilesAdaptsCoalescedWorkingSet(t *testing.T) {
	base := t.TempDir()
	trace := base + "/trace"
	pages := base + "/pages"
	require.NoError(t, os.WriteFile(trace, []byte(`{"version":1,"page_size":4,"offsets":[4,8]}`), 0600))
	require.NoError(t, os.WriteFile(pages, []byte("basepriv"), 0600))
	store := NewMemoryArtifactStore()
	repository, err := NewWorkingSetRepository(store, FixtureProvenance{4: BaseRootfsScope}, "base-v1")
	require.NoError(t, err)
	_, err = repository.PublishFiles(context.Background(), "revision-a", "image", trace, pages)
	require.NoError(t, err)
	source, err := NewProvenanceWorkingSetPageSource(context.Background(), store, "revision-a", nil)
	require.NoError(t, err)
	got, err := source.ReadAt(context.Background(), 4, 4)
	require.NoError(t, err)
	require.Equal(t, []byte("base"), got.Bytes)
	got, err = source.ReadAt(context.Background(), 8, 4)
	require.NoError(t, err)
	require.Equal(t, []byte("priv"), got.Bytes)
}

func TestRemotePublicationUsesProvenanceWorkingSetWhenEnabled(t *testing.T) {
	store := NewMemoryArtifactStore()
	manager := NewSnapshotManager(t.TempDir())
	manager.EnableRemoteTransfer(store, true)
	require.NoError(t, manager.EnableProvenanceWorkingSets(FixtureProvenance{0: BaseRootfsScope}, "base-v1"))
	snapshot, err := manager.InitSnapshot("revision-a", "registry.test/image:v1")
	require.NoError(t, err)
	require.NoError(t, snapshot.CreateSnapDir())
	require.NoError(t, os.WriteFile(snapshot.GetSnapshotFilePath(), []byte("state"), 0600))
	require.NoError(t, os.WriteFile(snapshot.GetMemFilePath(), []byte("memory"), 0600))
	require.NoError(t, snapshot.SerializeSnapInfo())
	require.NoError(t, os.WriteFile(snapshot.GetWorkingSetTraceFilePath(), []byte(`{"version":1,"page_size":4,"offsets":[0,4]}`), 0600))
	require.NoError(t, os.WriteFile(snapshot.GetWorkingSetFilePath(), []byte("basepriv"), 0600))
	require.NoError(t, manager.CommitSnapshot("revision-a"))
	require.NoError(t, manager.PublishSnapshot(context.Background(), "revision-a"))

	remote := newRemoteSnapshotTransfer(store, true)
	descriptor, err := remote.getDescriptor(context.Background(), "revision-a")
	require.NoError(t, err)
	require.True(t, descriptor.ProvenanceWorkingSet)
	source, err := NewProvenanceWorkingSetPageSource(context.Background(), store, "revision-a", nil)
	require.NoError(t, err)
	got, err := source.ReadAt(context.Background(), 4, 4)
	require.NoError(t, err)
	require.Equal(t, []byte("priv"), got.Bytes)
}

func TestProvenanceWorkingSetRejectsTruncatedSharedContent(t *testing.T) {
	store := NewMemoryArtifactStore()
	repository, err := NewWorkingSetRepository(store, FixtureProvenance{0: BaseRootfsScope}, "base-v1")
	require.NoError(t, err)
	manifest, err := repository.Publish(context.Background(), "revision-a", "image", 4, []WorkingSetPage{{PFN: 0, Bytes: []byte("base")}})
	require.NoError(t, err)
	require.NoError(t, store.Put(context.Background(), ArtifactKey(manifest.Base.Content), strings.NewReader("ba"), 2))
	_, err = NewProvenanceWorkingSetPageSource(context.Background(), store, "revision-a", nil)
	require.Error(t, err)
}

func TestContentHashProvenanceScopesBaseImageAndPrivateIDs(t *testing.T) {
	baseFile := t.TempDir() + "/base"
	imageFile := t.TempDir() + "/image"
	require.NoError(t, os.WriteFile(baseFile, []byte("base"), 0600))
	require.NoError(t, os.WriteFile(imageFile, []byte("imag"), 0600))
	policy, err := NewContentHashProvenance(4, []string{baseFile}, map[string][]string{"image-a": {imageFile}})
	require.NoError(t, err)
	require.Equal(t, BaseRootfsScope, policy.Classify(WorkingSetPage{Bytes: []byte("base")}, "image-a"))
	require.Equal(t, ImageScope, policy.Classify(WorkingSetPage{Bytes: []byte("imag")}, "image-a"))
	require.Equal(t, PrivateScope, policy.Classify(WorkingSetPage{Bytes: []byte("imag")}, "image-b"), "image bytes must be approved for the current image")

	store := NewMemoryArtifactStore()
	repository, err := NewWorkingSetRepository(store, policy, "base-v1")
	require.NoError(t, err)
	first, err := repository.Publish(context.Background(), "revision-a", "image-a", 4, []WorkingSetPage{{PFN: 0, Bytes: []byte("base")}, {PFN: 4, Bytes: []byte("imag")}, {PFN: 8, Bytes: []byte("priv")}})
	require.NoError(t, err)
	second, err := repository.Publish(context.Background(), "revision-b", "image-a", 4, []WorkingSetPage{{PFN: 8, Bytes: []byte("priv")}})
	require.NoError(t, err)
	require.Equal(t, pageHash([]byte("base")), first.Pages[0].Hash)
	require.Equal(t, pageHash([]byte("imag")), first.Pages[1].Hash)
	require.NotEqual(t, first.Pages[2].Hash, second.Pages[0].Hash, "private IDs are revision-scoped")
}
