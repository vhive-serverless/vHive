package snapshotting

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBaselineFixtureIsDeterministicAndLabelled(t *testing.T) {
	const pageSize = 16

	fixture := fixedMemoryFixture(pageSize)
	require.Equal(t, fixedMemoryFixture(pageSize), fixture)
	require.Equal(t, bytes.Repeat([]byte{'B'}, pageSize), fixture[:pageSize])
	require.Equal(t, bytes.Repeat([]byte{'I'}, pageSize), fixture[pageSize:2*pageSize])
	require.Equal(t, bytes.Repeat([]byte{'P'}, pageSize), fixture[2*pageSize:3*pageSize])
	require.Equal(t, fixture[:pageSize], fixture[3*pageSize:])
}

func TestBaselineFakeArtifactStoreCopiesDataAndPropagatesErrors(t *testing.T) {
	store := newFakeArtifactStore()
	original := []byte("snapshot-state")
	require.NoError(t, store.put("revision/state", original))

	original[0] = 'X'
	loaded, err := store.get("revision/state")
	require.NoError(t, err)
	require.Equal(t, []byte("snapshot-state"), loaded)

	loaded[0] = 'Y'
	loaded, err = store.get("revision/state")
	require.NoError(t, err)
	require.Equal(t, []byte("snapshot-state"), loaded)

	store.putError = errors.New("put failed")
	require.EqualError(t, store.put("revision/memory", []byte("memory")), "put failed")
	store.putError = nil
	store.getError = errors.New("get failed")
	_, err = store.get("revision/state")
	require.EqualError(t, err, "get failed")
}
