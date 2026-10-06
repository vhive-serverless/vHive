package snapshotting

import (
	"archive/tar"
	"bytes"
	"crypto/md5"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsolatedCacheLoadsSymlinkedImages(t *testing.T) {
	oldImages, oldRootfs := imageChunks, rootfsChunks
	t.Cleanup(func() { imageChunks, rootfsChunks = oldImages, oldRootfs })
	imageChunks = make(map[string]map[[md5.Size]byte]bool)
	root := t.TempDir()
	assets := filepath.Join(root, "assets")
	require.NoError(t, os.MkdirAll(filepath.Join(assets, "gate-image"), 0755))
	raw := bytes.Repeat([]byte{0x71}, 4096)
	for _, name := range []string{"gate-image/container.tar", "rootfs.tar"} {
		f, err := os.Create(filepath.Join(assets, name))
		require.NoError(t, err)
		w := tar.NewWriter(f)
		require.NoError(t, w.WriteHeader(&tar.Header{Name: "content", Mode: 0600, Size: int64(len(raw))}))
		_, err = w.Write(raw)
		require.NoError(t, err)
		require.NoError(t, w.Close())
		require.NoError(t, f.Close())
	}
	point := filepath.Join(root, "point")
	require.NoError(t, os.MkdirAll(point, 0755))
	require.NoError(t, os.Symlink(assets, filepath.Join(point, "images")))
	mgr := NewSnapshotManager(filepath.Join(point, "snapshots"), newMemoryRangeStorage(),
		true, false, true, true, true, false, 4096, 100, SecurityModePartial, 8, false, true, "")
	mgr.WaitForInit()
	require.True(t, imageChunks["gate-image"][md5.Sum(raw)])
	require.True(t, rootfsChunks[md5.Sum(raw)])
}
