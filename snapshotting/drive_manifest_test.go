package snapshotting

import (
	"os"
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
	require.NoError(t, os.WriteFile(manifest, []byte(completeDriveManifest), 0600))
	require.NoError(t, snapshot.ValidateDriveManifest())
	require.NoError(t, snapshot.Cleanup())
	_, err := os.Stat(manifest)
	require.ErrorIs(t, err, os.ErrNotExist, "snapshot cleanup must also remove its companion manifest")
}

// Match the runtime's versioned envelope and Go JSON field names inside its cut.
const completeDriveManifest = `{
 "version": 2,
 "drives": [
  {"id":"root_drive","path":"/old/rootfs.img","read_only":true,"root":true},
  {"id":"ctrstub0","path":"/old/workload.img","read_only":false,"root":false}
 ],
 "transport": {
  "Version":1,
  "Processes":[{"TaskID":"task","ExecID":"","Streams":[
   {"Name":"stdout","Sender":false,"Cursor":{"StreamID":"task-stdout","SessionID":"source-session","Offset":42,"FIN":false}}
  ]}],
  "EventCursor":7,
  "ResumeTasks":["task"]
 }
}`

func TestDriveManifestRequiresSealedTransport(t *testing.T) {
	for name, data := range map[string]string{
		"invalid JSON":           `{`,
		"old drive-only version": `{"version":1,"drives":[]}`,
		"missing cut":            `{"version":2,"drives":[]}`,
		"null cut":               `{"version":2,"drives":[],"transport":null}`,
		"unsupported cut":        `{"version":2,"drives":[],"transport":{"Version":2}}`,
		"unversioned cut":        `{"version":2,"drives":[],"transport":{}}`,
	} {
		t.Run(name, func(t *testing.T) {
			snapshot := NewSnapshot("revision", t.TempDir(), "image")
			require.NoError(t, snapshot.CreateSnapDir())
			require.NoError(t, os.WriteFile(snapshot.GetDriveManifestFilePath(), []byte(data), 0600))
			require.Error(t, snapshot.ValidateDriveManifest())
		})
	}
}
