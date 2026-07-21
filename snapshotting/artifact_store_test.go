package snapshotting

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemoryArtifactStoreRoundTripAndList(t *testing.T) {
	store := NewMemoryArtifactStore()
	first, err := RevisionArtifactKey("revision-a", "mem_file")
	require.NoError(t, err)
	second, err := SharedArtifactKey("chunks", "sha256-abc")
	require.NoError(t, err)
	require.NoError(t, store.Put(context.Background(), first, bytes.NewReader([]byte("memory-bytes")), int64(len("memory-bytes"))))
	require.NoError(t, store.Put(context.Background(), second, bytes.NewReader([]byte("chunk-bytes")), int64(len("chunk-bytes"))))

	reader, err := store.Get(context.Background(), first)
	require.NoError(t, err)
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	require.Equal(t, []byte("memory-bytes"), data)

	info, err := store.Stat(context.Background(), first)
	require.NoError(t, err)
	require.Equal(t, int64(len(data)), info.Size)
	objects, err := store.List(context.Background(), "revisions/")
	require.NoError(t, err)
	require.Equal(t, []ArtifactInfo{{Key: first, Size: int64(len(data)), ModTime: info.ModTime}}, objects)
}

func TestMemoryArtifactStorePropagatesFailuresAndMissingKey(t *testing.T) {
	store := NewMemoryArtifactStore()
	key, err := RevisionArtifactKey("revision-a", "mem_file")
	require.NoError(t, err)
	store.SetFailures(ArtifactStoreFailures{Put: errors.New("put failed")})
	require.EqualError(t, store.Put(context.Background(), key, bytes.NewReader(nil), 0), "put failed")

	store.SetFailures(ArtifactStoreFailures{Get: errors.New("get failed")})
	_, err = store.Get(context.Background(), key)
	require.EqualError(t, err, "get failed")
	store.SetFailures(ArtifactStoreFailures{Stat: errors.New("stat failed"), List: errors.New("list failed")})
	_, err = store.Stat(context.Background(), key)
	require.EqualError(t, err, "stat failed")
	_, err = store.List(context.Background(), "revisions/")
	require.EqualError(t, err, "list failed")
	store.SetFailures(ArtifactStoreFailures{})
	_, err = store.Get(context.Background(), key)
	require.ErrorIs(t, err, ErrArtifactNotFound)
}

func TestArtifactKeySchemesRejectAmbiguousParts(t *testing.T) {
	key, err := RevisionArtifactKey("revision-a", "mem_file")
	require.NoError(t, err)
	require.Equal(t, ArtifactKey("revisions/revision-a/mem_file"), key)
	key, err = SharedArtifactKey("chunks", "sha256-abc")
	require.NoError(t, err)
	require.Equal(t, ArtifactKey("shared/chunks/sha256-abc"), key)
	_, err = RevisionArtifactKey("../other", "mem_file")
	require.Error(t, err)
	_, err = SharedArtifactKey("chunks", "nested/object")
	require.Error(t, err)
}
