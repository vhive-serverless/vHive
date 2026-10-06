package ctriface

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestProcessVanishedAcceptsProcfsExitNotAccessFailure(t *testing.T) {
	for _, errno := range []error{syscall.ENOENT, syscall.ESRCH} {
		err := &os.PathError{Op: "read", Path: "/proc/128022/cmdline", Err: errno}
		if !processVanished(err) {
			t.Fatalf("concurrent exit rejected: %v", err)
		}
	}
	for _, errno := range []error{syscall.EACCES, syscall.EPERM, syscall.EIO} {
		err := &os.PathError{Op: "read", Path: "/proc/128022/cmdline", Err: errno}
		if processVanished(err) {
			t.Fatalf("access/read failure accepted as exit: %v", err)
		}
	}
	if processVanished(nil) {
		t.Fatal("successful read is not a concurrent exit")
	}
}

func TestPoolCleanupRetainsOnlyFailedMembers(t *testing.T) {
	var seen []string
	remaining, err := cleanupVMList(context.Background(), []string{"vm1", "vm2", "vm3"}, func(_ context.Context, id string) error {
		seen = append(seen, id)
		if id == "vm2" {
			return errors.New("VMM still running")
		}
		return nil
	})
	if err == nil || !reflect.DeepEqual(remaining, []string{"vm2"}) || len(seen) != 3 {
		t.Fatalf("remaining=%v seen=%v err=%v", remaining, seen, err)
	}
	remaining, err = cleanupVMList(context.Background(), remaining, func(context.Context, string) error { return nil })
	if err != nil || len(remaining) != 0 {
		t.Fatalf("retry remaining=%v err=%v", remaining, err)
	}
}

func TestPreparedVMStopsBeforeShimRemoval(t *testing.T) {
	for _, failure := range []string{"", "forced", "stop", "process", "remove"} {
		t.Run(failure, func(t *testing.T) {
			var order []string
			err := stopPreparedVM(context.Background(), "vm1", func(context.Context) error {
				order = append(order, "stop")
				if failure == "forced" {
					return status.Error(codes.Internal, "forcefully terminated VM vm1")
				}
				if failure == "stop" {
					return status.Error(codes.Unavailable, "control plane unreachable")
				}
				return nil
			}, func(string) error {
				order = append(order, "process")
				if failure == "process" {
					return errors.New("VMM still alive")
				}
				return nil
			}, func(context.Context) error {
				order = append(order, "remove")
				if failure == "remove" {
					return errors.New("remove failed")
				}
				return nil
			})
			want := []string{"stop", "process", "remove"}
			if failure == "stop" {
				want = want[:1]
			} else if failure == "process" {
				want = want[:2]
			}
			if !reflect.DeepEqual(order, want) || (err != nil) != (failure != "" && failure != "forced") {
				t.Fatalf("failure=%s order=%v err=%v", failure, order, err)
			}
		})
	}
}

func TestFailedVMReturnsResourcesOnlyAfterRemovalAndUFFD(t *testing.T) {
	for _, failure := range []string{"", "remove", "process", "uffd", "free"} {
		t.Run(failure, func(t *testing.T) {
			var order []string
			done := make(chan struct{})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if failure == "uffd" {
				cancel()
			} else {
				close(done)
			}
			err := cleanupFailedVM(ctx, "vm1", func(context.Context) error {
				order = append(order, "remove")
				if failure == "remove" {
					return errors.New("remove failed")
				}
				return nil
			}, done, func(string) error {
				order = append(order, "process")
				if failure == "process" {
					return errors.New("still alive")
				}
				return nil
			}, func() error {
				order = append(order, "free")
				if failure == "free" {
					return errors.New("network removal failed")
				}
				return nil
			})
			if (err != nil) != (failure != "") {
				t.Fatalf("failure=%s err=%v", failure, err)
			}
			want := []string{"remove", "process", "free"}
			if failure == "remove" {
				want = want[:1]
			}
			if failure == "process" || failure == "uffd" {
				want = want[:2]
			}
			if !reflect.DeepEqual(order, want) {
				t.Fatalf("order %v, want %v", order, want)
			}
		})
	}
}

func TestConfirmedTerminationRequiresProcessesAndUFFD(t *testing.T) {
	done := make(chan struct{})
	close(done)
	absent := func(string) error { return nil }
	forced := status.Error(codes.Internal, "forcefully terminated VM vm1")
	for _, stopErr := range []error{nil, forced} {
		if method, err := confirmVMTermination(context.Background(), "vm1", stopErr, done, absent); err != nil || method == "" {
			t.Fatalf("%s %v", method, err)
		}
	}
	for _, stopErr := range []error{status.Error(codes.Unavailable, "transport closed"),
		status.Error(codes.Internal, "forcefully terminated VM vm2"),
		status.Error(codes.Internal, "forcefully terminated VM vm1; wait failed")} {
		if _, err := confirmVMTermination(context.Background(), "vm1", stopErr, done, absent); err == nil {
			t.Fatal("accepted unconfirmed stop")
		}
	}
	if _, err := confirmVMTermination(context.Background(), "vm1", forced, done, func(string) error { return errors.New("still alive") }); err == nil {
		t.Fatal("accepted live VM")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := confirmVMTermination(ctx, "vm1", forced, make(chan struct{}), absent); err == nil {
		t.Fatal("accepted pending UFFD release")
	}
}

func TestVMProcessCheckMatchesExactOwnedProcesses(t *testing.T) {
	root := t.TempDir()
	pid := filepath.Join(root, "123")
	if err := os.Mkdir(pid, 0700); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"/usr/local/bin/firecracker\x00--id\x00vm1\x00", "/usr/local/bin/containerd-shim-aws-firecracker\x00-id\x00vm1\x00"} {
		if err := os.WriteFile(filepath.Join(pid, "cmdline"), []byte(command), 0600); err != nil {
			t.Fatal(err)
		}
		if err := vmProcessesAbsent(root, "vm1"); err == nil {
			t.Fatal("missed owned process")
		}
		if err := vmProcessesAbsent(root, "vm10"); err != nil {
			t.Fatal(err)
		}
	}
	if ownedVMCommand([]string{"bash", "--id", "vm1"}, "vm1") {
		t.Fatal("matched unrelated command")
	}
}
