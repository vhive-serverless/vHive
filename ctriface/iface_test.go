package ctriface

import (
	"context"
	"testing"
	"time"

	"github.com/containerd/containerd/namespaces"
	"github.com/stretchr/testify/require"
)

func TestStartStopSerial(t *testing.T) {
	testTimeout := 120 * time.Second
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), namespaceName), testTimeout)
	defer cancel()

	orch := NewOrchestrator("devmapper", 1)

	message, _, err := orch.StartVM(ctx, "test_vmID", "ustiugov/helloworld:runner_workload")
	require.NoError(t, err, "Failed to start VM, "+message)

	//message, err = orch.StopSingleVM(ctx, "test_vmID")
	//require.NoError(t, err, "Failed to stop VM, "+message)
}
