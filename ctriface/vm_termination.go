package ctriface

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Retain failed pool members so shutdown failure cannot erase ownership.
func cleanupVMList(ctx context.Context, ids []string, remove func(context.Context, string) error) ([]string, error) {
	var remaining []string
	var errs []error
	for _, id := range ids {
		if err := remove(ctx, id); err != nil {
			remaining = append(remaining, id)
			errs = append(errs, fmt.Errorf("VM %s: %w", id, err))
		}
	}
	return remaining, errors.Join(errs...)
}

// The pinned shim reports Internal after forceTerminate even when termination
// succeeds. Do not infer liveness from that status. Verify both exact VM-owned
// processes and this VM's UFFD release before freeing its network/backing state.
func confirmVMTermination(ctx context.Context, vmID string, stopErr error, uffdDone <-chan struct{}, absent func(string) error) (string, error) {
	method := "graceful"
	if stopErr != nil {
		if status.Code(stopErr) != codes.Internal || status.Convert(stopErr).Message() != "forcefully terminated VM "+vmID {
			return "", stopErr
		}
		method = "forced"
	}
	if err := absent(vmID); err != nil {
		return "", fmt.Errorf("VM termination not confirmed: %w", err)
	}
	if uffdDone != nil {
		select {
		case <-uffdDone:
		case <-ctx.Done():
			return "", fmt.Errorf("VM %s UFFD release pending: %w", vmID, ctx.Err())
		}
	}
	return method, nil
}

// Failed CreateVM can already own a connected UFFD and network. Remove the
// shim, establish absence/release, then return resources to their pools. An
// unconfirmed removal must leave the VM tracked, not free its network early.
func cleanupFailedVM(ctx context.Context, vmID string, remove func(context.Context) error, uffdDone <-chan struct{}, absent func(string) error, free func() error) error {
	if err := remove(ctx); err != nil {
		return fmt.Errorf("remove failed VM %s: %w", vmID, err)
	}
	if _, err := confirmVMTermination(ctx, vmID, nil, uffdDone, absent); err != nil {
		return err
	}
	return free()
}

// PrepareShim already starts a Firecracker process. Removing just the shim
// kills its control plane and orphans that process. Stop the VMM while its
// shim is reachable, establish termination, and only then remove metadata.
func stopPreparedVM(ctx context.Context, vmID string, stop func(context.Context) error, absent func(string) error, remove func(context.Context) error) error {
	if _, err := confirmVMTermination(ctx, vmID, stop(ctx), nil, absent); err != nil {
		return err
	}
	return remove(ctx)
}

func ownedVMCommand(args []string, vmID string) bool {
	if len(args) == 0 {
		return false
	}
	switch filepath.Base(args[0]) {
	case "firecracker", "containerd-shim-aws-firecracker":
	default:
		return false
	}
	for i, arg := range args {
		if (arg == "--id" || arg == "-id") && i+1 < len(args) && args[i+1] == vmID {
			return true
		}
		if arg == "--id="+vmID || arg == "-id="+vmID {
			return true
		}
	}
	return false
}

// procfs may report ESRCH, not only ENOENT, when a task exits between
// enumerating /proc and reading cmdline. Neither is a permissions failure.
func processVanished(err error) bool {
	return os.IsNotExist(err) || errors.Is(err, syscall.ESRCH)
}

func vmProcessesAbsent(procRoot, vmID string) error {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil || !entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(procRoot, entry.Name(), "cmdline"))
		if processVanished(err) { // A concurrently reaped process.
			continue
		}
		if err != nil {
			return err // Permission failures are not proof of absence.
		}
		if ownedVMCommand(strings.Split(string(data), "\x00"), vmID) {
			return fmt.Errorf("VM %s still has process %s", vmID, entry.Name())
		}
	}
	return nil
}
