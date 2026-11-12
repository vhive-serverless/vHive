package ctriface

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/containerd/fifo"
	"github.com/firecracker-microvm/firecracker-containerd/proto"
	"github.com/stretchr/testify/require"
	"github.com/vhive-serverless/vhive/misc"
)

type restoredOutputBuffer struct {
	sync.Mutex
	bytes.Buffer
}

func (b *restoredOutputBuffer) Write(data []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	return b.Buffer.Write(data)
}

func (b *restoredOutputBuffer) contents() string {
	b.Lock()
	defer b.Unlock()
	return b.Buffer.String()
}

func TestRestoredOutputDrainsTailBeforeClose(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	directory := t.TempDir()
	stdout, stderr := filepath.Join(directory, "stdout"), filepath.Join(directory, "stderr")
	require.NoError(t, syscall.Mkfifo(stdout, 0600))
	require.NoError(t, syscall.Mkfifo(stderr, 0600))
	vm := misc.NewVM("restored")
	output, errors := &restoredOutputBuffer{}, &restoredOutputBuffer{}
	description := &proto.RestoredProcessIO{ID: "function-task", Stdin: filepath.Join(directory, "stdin"), Stdout: stdout, Stderr: stderr}
	require.NoError(t, attachRestoredOutput(vm, []*proto.RestoredProcessIO{description}, output, errors))
	t.Cleanup(func() {
		for _, attachment := range vm.RestoredIOAttachments {
			attachment.Cancel()
			_ = attachment.Close()
		}
	})
	require.Len(t, vm.RestoredIO, 1)
	require.Equal(t, description.Stdin, vm.RestoredIO[0].Stdin)
	require.Empty(t, vm.RestoredIOAttachments[0].Config().Stdin, "a clone must not consume the source's external stdin")
	description.ID = "response-modified-later"
	require.Equal(t, "function-task", vm.RestoredIO[0].ID, "VM metadata must retain an independent response description")

	outWriter, err := fifo.OpenFifo(ctx, stdout, syscall.O_WRONLY|syscall.O_NONBLOCK, 0600)
	require.NoError(t, err)
	defer outWriter.Close()
	errWriter, err := fifo.OpenFifo(ctx, stderr, syscall.O_WRONLY|syscall.O_NONBLOCK, 0600)
	require.NoError(t, err)
	defer errWriter.Close()
	_, err = io.WriteString(outWriter, "stdout-prefix")
	require.NoError(t, err)
	_, err = io.WriteString(errWriter, "stderr-prefix")
	require.NoError(t, err)

	// A drain deadline must leave readers alive, preserving the remaining tail.
	stopCtx, stop := context.WithTimeout(ctx, 20*time.Millisecond)
	require.ErrorIs(t, closeRestoredOutput(stopCtx, vm), context.DeadlineExceeded)
	stop()
	_, err = io.WriteString(outWriter, "-tail")
	require.NoError(t, err)
	_, err = io.WriteString(errWriter, "-tail")
	require.NoError(t, err)
	require.NoError(t, outWriter.Close())
	require.NoError(t, errWriter.Close())
	require.NoError(t, closeRestoredOutput(ctx, vm))
	require.Equal(t, "stdout-prefix-tail", output.contents())
	require.Equal(t, "stderr-prefix-tail", errors.contents())
	_, err = os.Stat(stdout)
	require.NoError(t, err, "the runtime owns the FIFO paths; vHive only closes its readers")
}

func TestRestoredOutputRejectsMissingOrDuplicateFIFO(t *testing.T) {
	directory := t.TempDir()
	regular, pipe := filepath.Join(directory, "regular"), filepath.Join(directory, "pipe")
	require.NoError(t, os.WriteFile(regular, []byte("existing file"), 0600))
	require.NoError(t, syscall.Mkfifo(pipe, 0600))
	for name, streams := range map[string][]*proto.RestoredProcessIO{
		"nil description": {nil},
		"missing FIFO":    {{ID: "task", Stdout: filepath.Join(directory, "missing")}},
		"regular file":    {{ID: "task", Stdout: regular}},
		"duplicate FIFO":  {{ID: "task", Stdout: pipe, Stderr: pipe}},
	} {
		t.Run(name, func(t *testing.T) {
			vm := misc.NewVM("restored")
			require.Error(t, attachRestoredOutput(vm, streams, io.Discard, io.Discard))
			require.Empty(t, vm.RestoredIOAttachments)
		})
	}
	vm := misc.NewVM("restored")
	require.Error(t, attachRestoredOutput(vm, []*proto.RestoredProcessIO{{ID: "task", Stdout: pipe}}, nil, nil))
}
