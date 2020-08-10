package ctriface

import (
	"context"
	"flag"
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

var isUPFEnabled = flag.Bool("upf", false, "Set UPF enabled")

func TestSnapLoad(t *testing.T) {
	// Need to clean up manually after this test because StopVM does not
	// work for stopping machiens which are loaded from snapshots yet
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

	orch := NewOrchestrator("devmapper", WithTestModeOn(true), WithUPF(*isUPFEnabled))

	vmID := "1"

	message, _, err := orch.StartVM(ctx, vmID, "ustiugov/helloworld:runner_workload")
	require.NoError(t, err, "Failed to start VM, "+message)

	message, err = orch.PauseVM(ctx, vmID)
	require.NoError(t, err, "Failed to pause VM, "+message)

	message, err = orch.CreateSnapshot(ctx, vmID)
	require.NoError(t, err, "Failed to create snapshot of VM, "+message)

	message, _, err = orch.ResumeVM(ctx, vmID)
	require.NoError(t, err, "Failed to resume VM, "+message)

	message, err = orch.Offload(ctx, vmID)
	require.NoError(t, err, "Failed to offload VM, "+message)

	message, _, err = orch.LoadSnapshot(ctx, vmID)
	require.NoError(t, err, "Failed to load snapshot of VM, "+message)

	message, _, err = orch.ResumeVM(ctx, vmID)
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

	orch := NewOrchestrator("devmapper", WithTestModeOn(true), WithUPF(*isUPFEnabled))

	vmID := "3"

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

	message, err = orch.Offload(ctx, vmID)
	require.NoError(t, err, "Failed to offload VM, "+message)

	message, _, err = orch.LoadSnapshot(ctx, vmID)
	require.NoError(t, err, "Failed to load snapshot of VM, "+message)

	message, _, err = orch.ResumeVM(ctx, vmID)
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
	imageName := "ustiugov/helloworld:runner_workload"

	orch := NewOrchestrator("devmapper", WithTestModeOn(true), WithUPF(*isUPFEnabled))

	// Pull image
	_, err := orch.getImage(ctx, imageName)
	require.NoError(t, err, "Failed to pull image "+imageName)

	var vmGroup sync.WaitGroup
	for i := 0; i < vmNum; i++ {
		vmGroup.Add(1)
		go func(i int) {
			defer vmGroup.Done()
			vmID := fmt.Sprintf("%d", i+vmIDBase)

			message, _, err := orch.StartVM(ctx, vmID, "ustiugov/helloworld:runner_workload")
			require.NoError(t, err, "Failed to start VM, "+vmID+", "+message)

			message, err = orch.PauseVM(ctx, vmID)
			require.NoError(t, err, "Failed to pause VM, "+vmID+", "+message)

			message, err = orch.CreateSnapshot(ctx, vmID)
			require.NoError(t, err, "Failed to create snapshot of VM, "+vmID+", "+message)

			message, err = orch.Offload(ctx, vmID)
			require.NoError(t, err, "Failed to offload VM, "+vmID+", "+message)

			message, _, err = orch.LoadSnapshot(ctx, vmID)
			require.NoError(t, err, "Failed to load snapshot of VM, "+vmID+", "+message)

			message, _, err = orch.ResumeVM(ctx, vmID)
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
	imageName := "ustiugov/helloworld:runner_workload"

	orch := NewOrchestrator("devmapper", WithTestModeOn(true), WithUPF(*isUPFEnabled))

	// Pull image
	_, err := orch.getImage(ctx, imageName)
	require.NoError(t, err, "Failed to pull image "+imageName)

	{
		var vmGroup sync.WaitGroup
		for i := 0; i < vmNum; i++ {
			vmGroup.Add(1)
			go func(i int) {
				defer vmGroup.Done()
				vmID := fmt.Sprintf("%d", i+vmIDBase)
				message, _, err := orch.StartVM(ctx, vmID, imageName)
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
				message, err := orch.CreateSnapshot(ctx, vmID)
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

	{
		var vmGroup sync.WaitGroup
		for i := 0; i < vmNum; i++ {
			vmGroup.Add(1)
			go func(i int) {
				defer vmGroup.Done()
				vmID := fmt.Sprintf("%d", i+vmIDBase)
				message, _, err := orch.LoadSnapshot(ctx, vmID)
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
				message, _, err := orch.ResumeVM(ctx, vmID)
				require.NoError(t, err, "Failed to resume VM, "+vmID+", "+message)
			}(i)
		}
		vmGroup.Wait()
	}

	orch.Cleanup()
}
