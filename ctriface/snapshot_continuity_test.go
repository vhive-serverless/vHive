// MIT License
//
// Copyright (c) 2026 vHive team
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package ctriface

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	taskAPI "github.com/containerd/containerd/api/runtime/task/v3"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/ttrpcutil"
	"github.com/containerd/typeurl/v2"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/require"
	hello "github.com/vhive-serverless/vhive/examples/protobuf/helloworld"
	"github.com/vhive-serverless/vhive/snapshotting"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TestSnapshotSourceMemoryAndDiskIsolation uses vHive's legacy snapshot API,
// not CheckpointVM/native restore. The function keeps serving real requests;
// a pre-existing exec proves memory continuity independently of the stateless
// greeter response. Cloned tasks are runtime-owned, so inspection uses their
// shim task API rather than pretending that containerd adopted them.
func TestSnapshotSourceMemoryAndDiskIsolation(t *testing.T) {
	if *snapshotter != "devmapper" || *isUPFEnabled {
		t.Skip("legacy continuity acceptance requires devmapper and File memory snapshots")
	}
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), namespaceName), 5*time.Minute)
	defer cancel()
	base := filepath.Join("/fccd", "continuity-"+strconv.FormatInt(time.Now().UnixNano(), 36))
	orch := NewOrchestrator(*snapshotter, "", WithTestModeOn(true), WithSnapshotsDir(base), WithDockerCredentials(*dockerCredentials))
	t.Cleanup(func() {
		require.NoError(t, orch.StopActiveVMs())
		orch.Cleanup()
	})
	const sourceID, cloneID, counterID = "continuity-source", "continuity-clone", "memory-counter"
	source, _, err := orch.StartVM(ctx, sourceID, *testImage)
	require.NoError(t, err)
	legacyContinuityInvoke(ctx, t, source.GuestIP, "before-capture")
	sourceVM, err := orch.vmPool.GetVM(sourceID)
	require.NoError(t, err)
	require.NotNil(t, sourceVM.Task)
	require.NotNil(t, sourceVM.Image)
	t.Logf("function_image=%s digest=%s", *testImage, (*sourceVM.Image).Target().Digest)
	task := *sourceVM.Task
	backend, sourceDir := legacyContinuityTaskClient(t, sourceID)
	identity := legacyContinuityIdentity(t, sourceDir)
	legacyContinuityExec(ctx, t, backend, task.ID(), "mkdir -p /vhive-continuity; echo captured >/vhive-continuity/disk; sync")
	const counter = `
token=$(head -c 16 /dev/urandom | od -An -tx1 | tr -d ' \n')
counter=40
while :; do
    if test -f /vhive-continuity/request; then
        rm /vhive-continuity/request
        counter=$((counter+1))
        printf '%s:%s:%s\n' "$token" "$$" "$counter" >/vhive-continuity/response.next
        mv /vhive-continuity/response.next /vhive-continuity/response
    fi
    sleep .01
done`
	process, err := task.Exec(ctx, counterID, &specs.Process{Args: []string{"/bin/sh", "-ec", counter}, Cwd: "/"},
		cio.NewCreator(cio.WithStreams(nil, io.Discard, io.Discard)))
	require.NoError(t, err)
	require.NoError(t, process.Start(ctx))
	guestHostPID := process.Pid()
	require.Positive(t, guestHostPID)
	initial := legacyContinuityProbe(ctx, t, backend, task.ID())
	parts := strings.Split(initial, ":")
	require.Len(t, parts, 3)
	require.Len(t, parts[0], 32)
	// The shell reports its container PID; the task API reports its guest host PID.
	containerPID, err := strconv.ParseUint(parts[1], 10, 32)
	require.NoError(t, err)
	require.Positive(t, containerPID)
	require.Equal(t, "41", parts[2])
	next := parts[0] + ":" + parts[1] + ":"

	snapshot := snapshotting.NewSnapshot("continuity", orch.GetSnapshotsDir(), *testImage)
	require.NoError(t, snapshot.CreateSnapDir())
	require.NoError(t, orch.PauseVM(ctx, sourceID))
	require.NoError(t, orch.CreateSnapshot(ctx, sourceID, snapshot))
	_, err = orch.ResumeVM(ctx, sourceID)
	require.NoError(t, err)
	require.Equal(t, identity, legacyContinuityIdentity(t, sourceDir), "source VMM must not restart")
	require.Equal(t, next+"42", legacyContinuityProbe(ctx, t, backend, task.ID()), "source shell memory must continue rather than restart")
	legacyContinuityInvoke(ctx, t, source.GuestIP, "source-after-capture")
	legacyContinuityExec(ctx, t, backend, task.ID(), "echo source >/vhive-continuity/disk; sync")
	sourceDisk, err := orch.devMapper.GetDeviceSnapshot(ctx, sourceVM.ContainerSnapKey)
	require.NoError(t, err)

	clone, _, err := orch.LoadSnapshot(ctx, cloneID, snapshot)
	require.NoError(t, err)
	_, err = orch.ResumeVM(ctx, cloneID)
	require.NoError(t, err)
	require.NotEqual(t, source.GuestIP, clone.GuestIP)
	cloneVM, err := orch.vmPool.GetVM(cloneID)
	require.NoError(t, err)
	cloneDisk, err := orch.devMapper.GetDeviceSnapshot(ctx, cloneVM.ContainerSnapKey)
	require.NoError(t, err)
	require.NotEqual(t, sourceDisk.GetDevicePath(), cloneDisk.GetDevicePath())
	cloneBackend, cloneDir := legacyContinuityTaskClient(t, cloneID)
	require.NotEqual(t, identity.PID, legacyContinuityIdentity(t, cloneDir).PID)
	state, err := cloneBackend.State(ctx, &taskAPI.StateRequest{ID: task.ID(), ExecID: counterID})
	require.NoError(t, err)
	require.Equal(t, guestHostPID, state.Pid, "restore must retain the captured exec")
	require.Equal(t, next+"42", legacyContinuityProbe(ctx, t, cloneBackend, task.ID()), "clone must resume the captured token and counter, not the advanced source")
	require.Equal(t, "captured", legacyContinuityExec(ctx, t, cloneBackend, task.ID(), "cat /vhive-continuity/disk"))
	legacyContinuityExec(ctx, t, cloneBackend, task.ID(), "echo clone >/vhive-continuity/disk; sync")
	require.Equal(t, "source", legacyContinuityExec(ctx, t, backend, task.ID(), "cat /vhive-continuity/disk"))
	require.Equal(t, "clone", legacyContinuityExec(ctx, t, cloneBackend, task.ID(), "cat /vhive-continuity/disk"))
	require.Equal(t, next+"43", legacyContinuityProbe(ctx, t, backend, task.ID()))
	require.Equal(t, next+"43", legacyContinuityProbe(ctx, t, cloneBackend, task.ID()))
	legacyContinuityInvoke(ctx, t, source.GuestIP, "live-source-with-clone")
	legacyContinuityInvoke(ctx, t, clone.GuestIP, "live-clone-with-source")

	// Delete all source backing state through vHive's normal lifecycle. The
	// clone must retain both its memory and disk independently of that cleanup.
	require.NoError(t, orch.StopSingleVM(ctx, sourceID))
	_, err = os.Stat(sourceDir)
	require.True(t, os.IsNotExist(err), "source shim directory must be gone")
	require.Equal(t, next+"44", legacyContinuityProbe(ctx, t, cloneBackend, task.ID()))
	require.Equal(t, "clone", legacyContinuityExec(ctx, t, cloneBackend, task.ID(), "cat /vhive-continuity/disk"))
	legacyContinuityInvoke(ctx, t, clone.GuestIP, "clone-after-source-delete")
	require.NoError(t, orch.StopSingleVM(ctx, cloneID))
	t.Logf("legacy_snapshot_continuity source_vmm=%d guest_exec_host_pid=%d container_exec_pid=%d initial=%s source_and_clone_disk_isolation=passed source_deleted=true", identity.PID, guestHostPID, containerPID, initial)
}

func legacyContinuityInvoke(ctx context.Context, t *testing.T, ip, payload string) {
	t.Helper()
	connection, err := grpc.NewClient(net.JoinHostPort(ip, "50051"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() { _ = connection.Close() }()
	request, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	reply, err := hello.NewGreeterClient(connection).SayHello(request, &hello.HelloRequest{Name: payload}, grpc.WaitForReady(true))
	require.NoError(t, err)
	require.Equal(t, "Hello, "+payload+"!", reply.GetMessage())
}

func legacyContinuityTaskClient(t *testing.T, vmID string) (taskAPI.TTRPCTaskService, string) {
	t.Helper()
	// Read the activated configuration; do not import runtime internal packages.
	data, err := os.ReadFile("/etc/containerd/firecracker-runtime.json")
	require.NoError(t, err)
	var config struct {
		ShimBaseDir string `json:"shim_base_dir"`
	}
	require.NoError(t, json.Unmarshal(data, &config))
	if config.ShimBaseDir == "" {
		config.ShimBaseDir = "/var/lib/firecracker-containerd/shim-base"
	}
	directory := filepath.Join(config.ShimBaseDir, namespaceName+"#"+vmID)
	address, err := os.ReadFile(filepath.Join(directory, "address"))
	require.NoError(t, err)
	connection, err := ttrpcutil.NewClient(string(address))
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	rpc, err := connection.Client()
	require.NoError(t, err)
	return taskAPI.NewTTRPCTaskClient(rpc), directory
}

type legacyContinuityProcess struct {
	PID        int    `json:"pid"`
	StartTicks string `json:"start_ticks"`
}

func legacyContinuityIdentity(t *testing.T, directory string) legacyContinuityProcess {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(directory, "recovery.json"))
	require.NoError(t, err)
	var recovery struct {
		VMM legacyContinuityProcess `json:"vmm"`
	}
	require.NoError(t, json.Unmarshal(data, &recovery))
	require.Positive(t, recovery.VMM.PID)
	actual, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", recovery.VMM.PID))
	require.NoError(t, err)
	end := strings.LastIndexByte(string(actual), ')')
	require.NotEqual(t, -1, end)
	fields := strings.Fields(string(actual[end+1:]))
	require.GreaterOrEqual(t, len(fields), 20)
	require.Equal(t, recovery.VMM.StartTicks, fields[19], "recorded VMM identity must still be alive")
	return recovery.VMM
}

func legacyContinuityProbe(ctx context.Context, t *testing.T, backend taskAPI.TTRPCTaskService, taskID string) string {
	t.Helper()
	return legacyContinuityExec(ctx, t, backend, taskID, "rm -f /vhive-continuity/response; touch /vhive-continuity/request; while ! test -f /vhive-continuity/response; do sleep .01; done; cat /vhive-continuity/response")
}

func legacyContinuityExec(ctx context.Context, t *testing.T, backend taskAPI.TTRPCTaskService, taskID, command string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	execID := fmt.Sprintf("continuity-probe-%d", time.Now().UnixNano())
	streams, err := cio.NewCreator(cio.WithStreams(nil, &stdout, &stderr))(execID)
	require.NoError(t, err)
	defer func() { _ = streams.Close() }()
	config := streams.Config()
	spec, err := typeurl.MarshalAnyToProto(&specs.Process{Args: []string{"/bin/sh", "-ec", command}, Cwd: "/"})
	require.NoError(t, err)
	_, err = backend.Exec(ctx, &taskAPI.ExecProcessRequest{ID: taskID, ExecID: execID, Spec: spec, Stdin: config.Stdin, Stdout: config.Stdout, Stderr: config.Stderr})
	require.NoError(t, err)
	_, err = backend.Start(ctx, &taskAPI.StartRequest{ID: taskID, ExecID: execID})
	require.NoError(t, err)
	result, err := backend.Wait(ctx, &taskAPI.WaitRequest{ID: taskID, ExecID: execID})
	require.NoError(t, err)
	require.Zero(t, result.ExitStatus)
	_, err = backend.Delete(ctx, &taskAPI.DeleteRequest{ID: taskID, ExecID: execID})
	require.NoError(t, err)
	streams.Wait()
	require.Empty(t, stderr.String())
	return strings.TrimSpace(stdout.String())
}
