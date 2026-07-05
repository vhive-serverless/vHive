package ctriface

import (
	"testing"

	fcproto "github.com/firecracker-microvm/firecracker-containerd/proto"
	"github.com/stretchr/testify/require"
	googleproto "google.golang.org/protobuf/proto"
)

func TestCreateVMRequestWithMemoryBackendMarshalsForTTRPC(t *testing.T) {
	req := &fcproto.CreateVMRequest{
		VMID:         "vm-with-upf",
		LoadSnapshot: true,
		MemBackend: &fcproto.MemoryBackend{
			BackendType: "Uffd",
			BackendPath: "/tmp/vhive-upf.sock",
		},
	}

	payload, err := googleproto.Marshal(req)
	require.NoError(t, err)
	require.NotEmpty(t, payload)

	var decoded fcproto.CreateVMRequest
	require.NoError(t, googleproto.Unmarshal(payload, &decoded))
	require.Equal(t, "Uffd", decoded.GetMemBackend().GetBackendType())
	require.Equal(t, "/tmp/vhive-upf.sock", decoded.GetMemBackend().GetBackendPath())
}

func TestConfigureSnapshotMemoryBackendClearsLegacyMemFilePath(t *testing.T) {
	req := &fcproto.CreateVMRequest{
		MemFilePath: "/fccd/snapshots/myrev/mem_file",
	}

	configureSnapshotMemoryBackend(req, "File", "/fccd/snapshots/myrev/mem_file")

	require.Empty(t, req.GetMemFilePath())
	require.Equal(t, "File", req.GetMemBackend().GetBackendType())
	require.Equal(t, "/fccd/snapshots/myrev/mem_file", req.GetMemBackend().GetBackendPath())
}

func TestCreateVMRequestWithFileMemoryBackendMarshalsForTTRPC(t *testing.T) {
	req := &fcproto.CreateVMRequest{
		VMID:         "vm-with-file-backend",
		LoadSnapshot: true,
	}
	configureSnapshotMemoryBackend(req, "File", "/fccd/snapshots/myrev/mem_file")

	payload, err := googleproto.Marshal(req)
	require.NoError(t, err)
	require.NotEmpty(t, payload)

	var decoded fcproto.CreateVMRequest
	require.NoError(t, googleproto.Unmarshal(payload, &decoded))
	require.Empty(t, decoded.GetMemFilePath())
	require.Equal(t, "File", decoded.GetMemBackend().GetBackendType())
	require.Equal(t, "/fccd/snapshots/myrev/mem_file", decoded.GetMemBackend().GetBackendPath())
}
