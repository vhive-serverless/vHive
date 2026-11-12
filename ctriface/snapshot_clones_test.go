package ctriface

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/stretchr/testify/require"
	hello "github.com/vhive-serverless/vhive/examples/protobuf/helloworld"
	"github.com/vhive-serverless/vhive/snapshotting"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TestSnapshotConcurrentClones uses the Step 2 Pause/CreateSnapshot/Resume API.
// It requires an isolated installed stack and the helloworld function image.
func TestSnapshotConcurrentClones(t *testing.T) {
	if *snapshotter != "devmapper" || *isUPFEnabled {
		t.Skip("concurrent clone acceptance requires devmapper and File memory snapshots")
	}
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), namespaceName), 3*time.Minute)
	defer cancel()
	orch := NewOrchestrator(*snapshotter, "", WithTestModeOn(true), WithDockerCredentials(*dockerCredentials))
	t.Cleanup(func() {
		require.NoError(t, orch.StopActiveVMs())
		orch.Cleanup()
	})

	invoke := func(ip, payload string) {
		t.Helper()
		connection, err := grpc.NewClient(net.JoinHostPort(ip, "50051"), grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)
		defer connection.Close()
		requestCtx, stop := context.WithTimeout(ctx, 20*time.Second)
		defer stop()
		reply, err := hello.NewGreeterClient(connection).SayHello(requestCtx, &hello.HelloRequest{Name: payload}, grpc.WaitForReady(true))
		require.NoError(t, err)
		require.Equal(t, "Hello, "+payload+"!", reply.GetMessage())
	}

	const sourceID = "concurrent-source"
	source, _, err := orch.StartVM(ctx, sourceID, *testImage)
	require.NoError(t, err)
	invoke(source.GuestIP, "before-snapshot")
	snapshot := snapshotting.NewSnapshot("concurrent-clones", orch.GetSnapshotsDir(), *testImage)
	require.NoError(t, snapshot.CreateSnapDir())
	require.NoError(t, orch.PauseVM(ctx, sourceID))
	require.NoError(t, orch.CreateSnapshot(ctx, sourceID, snapshot))
	_, err = orch.ResumeVM(ctx, sourceID)
	require.NoError(t, err)
	invoke(source.GuestIP, "source-after-snapshot")

	addresses := map[string]bool{source.GuestIP: true}
	disks := make(map[string]bool)
	sourceVM, err := orch.vmPool.GetVM(sourceID)
	require.NoError(t, err)
	sourceDisk, err := orch.devMapper.GetDeviceSnapshot(ctx, sourceVM.ContainerSnapKey)
	require.NoError(t, err)
	disks[sourceDisk.GetDevicePath()] = true
	clones := make(map[string]string)
	for _, id := range []string{"concurrent-clone-one", "concurrent-clone-two"} {
		clone, _, err := orch.LoadSnapshot(ctx, id, snapshot)
		require.NoError(t, err)
		_, err = orch.ResumeVM(ctx, id)
		require.NoError(t, err)
		require.False(t, addresses[clone.GuestIP], "each live instance needs an independent host endpoint")
		addresses[clone.GuestIP] = true
		vm, err := orch.vmPool.GetVM(id)
		require.NoError(t, err)
		disk, err := orch.devMapper.GetDeviceSnapshot(ctx, vm.ContainerSnapKey)
		require.NoError(t, err)
		require.False(t, disks[disk.GetDevicePath()], "source and clones must have independent writable disks")
		disks[disk.GetDevicePath()] = true
		clones[id] = clone.GuestIP
		invoke(clone.GuestIP, id)
	}
	invoke(source.GuestIP, "source-with-two-clones")
	require.NoError(t, orch.StopSingleVM(ctx, "concurrent-clone-one"))
	invoke(source.GuestIP, "source-after-clone-stop")
	invoke(clones["concurrent-clone-two"], "sibling-after-clone-stop")
	require.NoError(t, orch.StopSingleVM(ctx, sourceID))
	invoke(clones["concurrent-clone-two"], "clone-after-source-stop")
	require.NoError(t, orch.StopSingleVM(ctx, "concurrent-clone-two"))
}
