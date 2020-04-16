package ctriface

/*
import (
	"context"
	"testing"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/pkg/ttrpcutil"
	"github.com/stretchr/testify/require"
)

func TestStartStopSerial(t *testing.T) {
	prepareIntegTest(t)
	testTimeout := 120 * time.Second
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), defaultNamespace), testTimeout)
	defer cancel()

	client, err := containerd.New(containerdSockPath, containerd.WithDefaultRuntime(firecrackerRuntime))
	require.NoError(t, err, "unable to create client to containerd service at %s, is containerd running?", containerdSockPath)
	defer client.Close()

	pluginClient, err := ttrpcutil.NewClient(containerdSockPath + ".ttrpc")
	require.NoError(t, err, "failed to create ttrpc client")

	image, err := alpineImage(ctx, client, defaultSnapshotterName)
	require.NoError(t, err, "failed to get alpine image")

	orch = NewOrchestrator("devmapper", 1)

	message, _, err := orch.StartVM(ctx, "test_vmID", "ustiugov/helloworld:runner_workload")
	require.NoError(t, err, "Failed to start VM, "+message)

	message, err = orch.StopSingleVM(ctx, "test_vmID")
	require.NoError(t, err, "Failed to stop VM, "+message)
}
*/
