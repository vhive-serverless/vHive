package ctriface

import (
	"context"
	"os"
	"testing"
	"time"

	ctrdlog "github.com/containerd/containerd/log"
	"github.com/containerd/containerd/namespaces"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestStartSnapStop(t *testing.T) {
	// BROKEN BECAUSE StopVM does not work yet.
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)

	log.SetLevel(log.DebugLevel)

	testTimeout := 120 * time.Second
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), namespaceName), testTimeout)
	defer cancel()

	orch := NewOrchestrator("devmapper", WithTestModeOn(true))

	vmID := "2"

	message, _, err := orch.StartVM(ctx, vmID, "ustiugov/helloworld:runner_workload")
	require.NoError(t, err, "Failed to start VM, "+message)

	message, err = orch.PauseVM(ctx, vmID)
	require.NoError(t, err, "Failed to pause VM, "+message)

	message, err = orch.CreateSnapshot(ctx, vmID)
	require.NoError(t, err, "Failed to create snapshot of VM, "+message)

	message, err = orch.Offload(ctx, vmID)
	require.NoError(t, err, "Failed to offload VM, "+message)

	message, _, err = orch.LoadSnapshot(ctx, vmID)
	require.NoError(t, err, "Failed to load snapshot of VM, "+message)

	message, _, err = orch.ResumeVM(ctx, vmID)
	require.NoError(t, err, "Failed to resume VM, "+message)

	message, err = orch.StopSingleVMOnly(ctx, vmID)
	require.NoError(t, err, "Failed to stop VM, "+message)

	orch.Cleanup()
}
