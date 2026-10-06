package storage

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCountingReadCloserCountsOneRequestAndDeliveredBytes(t *testing.T) {
	counter := &remoteFetchCounter{}
	reader := &countingReadCloser{
		inner:   io.NopCloser(bytes.NewReader([]byte("abcdef"))),
		counter: counter,
	}

	first := make([]byte, 2)
	n, err := reader.Read(first)
	require.NoError(t, err)
	require.Equal(t, 2, n)

	rest, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, []byte("cdef"), rest)
	require.NoError(t, reader.Close())
	require.Equal(t, uint64(1), counter.requests.Load())
	require.Equal(t, uint64(6), counter.bytes.Load())
}

func TestMinioStorageFetchStatsClassifySnapshotAndReset(t *testing.T) {
	store := &MinioStorage{}
	store.countCompletedObjectRead("_chunks_zstd_v1_l3/aa/hash", 4096)
	store.countCompletedObjectRead("revision/recipe_file", 8192)
	store.countCompletedObjectRead("revision/snap_file", 1024)
	store.countCompletedObjectRead("revision/working_set_pages_content.zstd.frames", 16384)

	stats := store.SnapshotRemoteFetchStats()
	require.Equal(t, RemoteFetchClassStats{Requests: 4, Bytes: 29696}, stats.Total)
	require.Equal(t, RemoteFetchClassStats{Requests: 1, Bytes: 4096}, stats.Classes["chunks"])
	require.Equal(t, RemoteFetchClassStats{Requests: 1, Bytes: 8192}, stats.Classes["recipe"])
	require.Equal(t, RemoteFetchClassStats{Requests: 1, Bytes: 1024}, stats.Classes["snapshot_metadata"])
	require.Equal(t, RemoteFetchClassStats{Requests: 1, Bytes: 16384}, stats.Classes["working_set_payload"])

	store.ResetRemoteFetchStats()
	require.Equal(t, RemoteFetchClassStats{}, store.SnapshotRemoteFetchStats().Total)
}

func TestRemoteFetchClass(t *testing.T) {
	cases := map[string]string{
		"revision/working_set_pages_content.zstd.streams":              "working_set_payload",
		"revision/working_set_pages_content_private.zstd.streams":      "working_set_payload",
		"revision/working_set_pages_content.zstd.streams.json":         "working_set_metadata",
		"revision/working_set_pages_content_private.zstd.streams.json": "working_set_metadata",
		"_chunks_zstd_v1_l3/aa/hash":                                   "chunks",
		"revision/working_set_pages_content_private.zstd.frames":       "working_set_payload",
		"revision/working_set_pages_content_private.zstd.json":         "working_set_metadata",
		"revision/working_set_pages_index_private":                     "working_set_metadata",
		"revision/working_set_pages":                                   "working_set_metadata",
		"revision/recipe_file":                                         "recipe",
		"revision/info_file":                                           "snapshot_metadata",
		"ws_shared/base/content":                                       "shared_working_set",
		"revision/unknown":                                             "other",
	}
	for objectKey, expected := range cases {
		t.Run(objectKey, func(t *testing.T) {
			require.Equal(t, expected, remoteFetchClassNames[remoteFetchClass(objectKey)])
		})
	}
}
