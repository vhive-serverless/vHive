package snapshotting

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDriveManifestLifecycle(t *testing.T) {
	snapshot := NewSnapshot("revision", t.TempDir(), "image")
	require.NoError(t, snapshot.CreateSnapDir())
	require.Error(t, snapshot.ValidateDriveManifest(), "old snapshots must fail before restore allocates resources")
	manifest := snapshot.GetDriveManifestFilePath()
	require.Equal(t, snapshot.GetSnapshotFilePath()+".drives.json", manifest)
	require.NoError(t, os.WriteFile(manifest, nil, 0600))
	require.Error(t, snapshot.ValidateDriveManifest(), "an incomplete capture must not become ready")
	require.NoError(t, os.WriteFile(manifest, []byte(`{"version":1,"drives":[]}`), 0600))
	require.NoError(t, snapshot.ValidateDriveManifest())
	require.Equal(t, filepath.Dir(snapshot.GetSnapshotFilePath()), filepath.Dir(manifest))
	require.NoError(t, snapshot.Cleanup())
	_, err := os.Stat(manifest)
	require.ErrorIs(t, err, os.ErrNotExist, "snapshot cleanup must also remove its companion manifest")
}
