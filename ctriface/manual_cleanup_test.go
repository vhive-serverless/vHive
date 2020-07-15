package ctriface

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	ctrdlog "github.com/containerd/containerd/log"
	"github.com/containerd/containerd/namespaces"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestSnapLoad(t *testing.T) {
	// Need to clean up manually after this test because StopVM does not
	// work for stopping machiens which are loaded from snapshots yet
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)

	log.SetLevel(log.InfoLevel)

	testTimeout := 120 * time.Second
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), namespaceName), testTimeout)
	defer cancel()

	orch := NewOrchestrator("devmapper", 1, true)

	vmID := "1"

	message, _, err := orch.StartVM(ctx, vmID, "ustiugov/helloworld:runner_workload")
	require.NoError(t, err, "Failed to start VM, "+message)

	message, err = orch.PauseVM(ctx, vmID)
	require.NoError(t, err, "Failed to pause VM, "+message)

	message, err = orch.CreateSnapshot(ctx, vmID, "/tmp/snapshot_file", "/tmp/mem_file")
	require.NoError(t, err, "Failed to create snapshot of VM, "+message)

	message, err = orch.ResumeVM(ctx, vmID)
	require.NoError(t, err, "Failed to resume VM, "+message)

	message, err = orch.Offload(ctx, vmID)
	require.NoError(t, err, "Failed to offload VM, "+message)

	time.Sleep(300 * time.Millisecond)

	message, err = orch.LoadSnapshot(ctx, vmID, "/tmp/snapshot_file", "/tmp/mem_file", false)
	require.NoError(t, err, "Failed to load snapshot of VM, "+message)

	message, err = orch.ResumeVM(ctx, vmID)
	require.NoError(t, err, "Failed to resume VM, "+message)

	orch.Cleanup()
}

func TestSnapLoadMultiple(t *testing.T) {
	// Needs to be cleaned up manually.
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)

	log.SetLevel(log.InfoLevel)

	testTimeout := 120 * time.Second
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), namespaceName), testTimeout)
	defer cancel()

	orch := NewOrchestrator("devmapper", 1, true)

	vmID := "3"

	message, _, err := orch.StartVM(ctx, vmID, "ustiugov/helloworld:runner_workload")
	require.NoError(t, err, "Failed to start VM, "+message)

	message, err = orch.PauseVM(ctx, vmID)
	require.NoError(t, err, "Failed to pause VM, "+message)

	message, err = orch.CreateSnapshot(ctx, vmID, "/tmp/snapshot_file1", "/tmp/mem_file1")
	require.NoError(t, err, "Failed to create snapshot of VM, "+message)

	message, err = orch.Offload(ctx, vmID)
	require.NoError(t, err, "Failed to offload VM, "+message)

	time.Sleep(300 * time.Millisecond)

	message, err = orch.LoadSnapshot(ctx, vmID, "/tmp/snapshot_file1", "/tmp/mem_file1", false)
	require.NoError(t, err, "Failed to load snapshot of VM, "+message)

	message, err = orch.ResumeVM(ctx, vmID)
	require.NoError(t, err, "Failed to resume VM, "+message)

	message, err = orch.Offload(ctx, vmID)
	require.NoError(t, err, "Failed to offload VM, "+message)

	time.Sleep(300 * time.Millisecond)

	message, err = orch.LoadSnapshot(ctx, vmID, "/tmp/snapshot_file1", "/tmp/mem_file1", false)
	require.NoError(t, err, "Failed to load snapshot of VM, "+message)

	message, err = orch.ResumeVM(ctx, vmID)
	require.NoError(t, err, "Failed to resume VM, "+message)

	message, err = orch.Offload(ctx, vmID)
	require.NoError(t, err, "Failed to offload VM, "+message)

	orch.Cleanup()
}

func TestParallelSnapLoad(t *testing.T) {
	// Needs to be cleaned up manually.
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)

	log.SetLevel(log.InfoLevel)

	testTimeout := 120 * time.Second
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), namespaceName), testTimeout)
	defer cancel()

	vmNum := 5
	vmIDBase := 6
	orch := NewOrchestrator("devmapper", vmNum, true)

	// Pull image to work around parallel pulling
	message, _, err := orch.StartVM(ctx, "img_plr", "ustiugov/helloworld:runner_workload")
	require.NoError(t, err, "Failed to start VM, "+message)

	message, err = orch.StopSingleVM(ctx, "img_plr")
	require.NoError(t, err, "Failed to stop VM, "+message)

	var vmGroup sync.WaitGroup
	for i := 0; i < vmNum; i++ {
		vmGroup.Add(1)
		go func(i int) {
			defer vmGroup.Done()
			vmID := fmt.Sprintf("%d", i+vmIDBase)
			snapshotFilePath := fmt.Sprintf("/dev/snapshot_file_%s", vmID)
			memFilePath := fmt.Sprintf("/dev/mem_file_%s", vmID)

			message, _, err := orch.StartVM(ctx, vmID, "ustiugov/helloworld:runner_workload")
			require.NoError(t, err, "Failed to start VM, "+vmID+", "+message)

			message, err = orch.PauseVM(ctx, vmID)
			require.NoError(t, err, "Failed to pause VM, "+vmID+", "+message)

			message, err = orch.CreateSnapshot(ctx, vmID, snapshotFilePath, memFilePath)
			require.NoError(t, err, "Failed to create snapshot of VM, "+vmID+", "+message)

			message, err = orch.Offload(ctx, vmID)
			require.NoError(t, err, "Failed to offload VM, "+vmID+", "+message)

			time.Sleep(300 * time.Millisecond)

			message, err = orch.LoadSnapshot(ctx, vmID, snapshotFilePath, memFilePath, false)
			require.NoError(t, err, "Failed to load snapshot of VM, "+vmID+", "+message)

			message, err = orch.ResumeVM(ctx, vmID)
			require.NoError(t, err, "Failed to resume VM, "+vmID+", "+message)
		}(i)
	}
	vmGroup.Wait()

	orch.Cleanup()
}

func TestParallelPhasedSnapLoad(t *testing.T) {
	// Needs to be cleaned up manually.
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)

	log.SetLevel(log.InfoLevel)

	testTimeout := 120 * time.Second
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), namespaceName), testTimeout)
	defer cancel()

	vmNum := 10
	vmIDBase := 11
	orch := NewOrchestrator("devmapper", vmNum, true)

	// Pull image to work around parallel pulling
	message, _, err := orch.StartVM(ctx, "img_plr", "ustiugov/helloworld:runner_workload")
	require.NoError(t, err, "Failed to start VM, "+message)

	message, err = orch.StopSingleVM(ctx, "img_plr")
	require.NoError(t, err, "Failed to stop VM, "+message)

	{
		var vmGroup sync.WaitGroup
		for i := 0; i < vmNum; i++ {
			vmGroup.Add(1)
			go func(i int) {
				defer vmGroup.Done()
				vmID := fmt.Sprintf("%d", i+vmIDBase)
				message, _, err := orch.StartVM(ctx, vmID, "ustiugov/helloworld:runner_workload")
				require.NoError(t, err, "Failed to start VM, "+vmID+", "+message)
			}(i)
		}
		vmGroup.Wait()
	}

	{
		var vmGroup sync.WaitGroup
		for i := 0; i < vmNum; i++ {
			vmGroup.Add(1)
			go func(i int) {
				defer vmGroup.Done()
				vmID := fmt.Sprintf("%d", i+vmIDBase)
				message, err := orch.PauseVM(ctx, vmID)
				require.NoError(t, err, "Failed to pause VM, "+vmID+", "+message)
			}(i)
		}
		vmGroup.Wait()
	}

	{
		var vmGroup sync.WaitGroup
		for i := 0; i < vmNum; i++ {
			vmGroup.Add(1)
			go func(i int) {
				defer vmGroup.Done()
				vmID := fmt.Sprintf("%d", i+vmIDBase)
				snapshotFilePath := fmt.Sprintf("/dev/snapshot_file_%s", vmID)
				memFilePath := fmt.Sprintf("/dev/mem_file_%s", vmID)
				message, err := orch.CreateSnapshot(ctx, vmID, snapshotFilePath, memFilePath)
				require.NoError(t, err, "Failed to create snapshot of VM, "+vmID+", "+message)
			}(i)
		}
		vmGroup.Wait()
	}

	{
		var vmGroup sync.WaitGroup
		for i := 0; i < vmNum; i++ {
			vmGroup.Add(1)
			go func(i int) {
				defer vmGroup.Done()
				vmID := fmt.Sprintf("%d", i+vmIDBase)
				message, err := orch.Offload(ctx, vmID)
				require.NoError(t, err, "Failed to offload VM, "+vmID+", "+message)
			}(i)
		}
		vmGroup.Wait()
	}

	time.Sleep(300 * time.Millisecond)

	{
		var vmGroup sync.WaitGroup
		for i := 0; i < vmNum; i++ {
			vmGroup.Add(1)
			go func(i int) {
				defer vmGroup.Done()
				vmID := fmt.Sprintf("%d", i+vmIDBase)
				snapshotFilePath := fmt.Sprintf("/dev/snapshot_file_%s", vmID)
				memFilePath := fmt.Sprintf("/dev/mem_file_%s", vmID)
				message, err := orch.LoadSnapshot(ctx, vmID, snapshotFilePath, memFilePath, false)
				require.NoError(t, err, "Failed to load snapshot of VM, "+vmID+", "+message)
			}(i)
		}
		vmGroup.Wait()
	}

	{
		var vmGroup sync.WaitGroup
		for i := 0; i < vmNum; i++ {
			vmGroup.Add(1)
			go func(i int) {
				defer vmGroup.Done()
				vmID := fmt.Sprintf("%d", i+vmIDBase)
				message, err := orch.ResumeVM(ctx, vmID)
				require.NoError(t, err, "Failed to resume VM, "+vmID+", "+message)
			}(i)
		}
		vmGroup.Wait()
	}

	orch.Cleanup()
}
