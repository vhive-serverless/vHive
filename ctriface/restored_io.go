package ctriface

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/firecracker-microvm/firecracker-containerd/proto"
	"github.com/vhive-serverless/vhive/misc"
	googleproto "google.golang.org/protobuf/proto"
)

// attachRestoredOutput supplies real log readers for runtime-adopted processes.
// External unread stdin remains source-owned and is not duplicated into clones.
func attachRestoredOutput(vm *misc.VM, streams []*proto.RestoredProcessIO, stdout, stderr io.Writer) error {
	seen := make(map[string]bool)
	for _, process := range streams {
		if process == nil {
			return errors.New("restore returned a nil process I/O description")
		}
		if process.Stdout != "" && stdout == nil || !process.Terminal && process.Stderr != "" && stderr == nil {
			return errors.New("restored process output requires a log writer")
		}
		paths := []string{process.Stdout}
		if !process.Terminal {
			paths = append(paths, process.Stderr)
		}
		for _, path := range paths {
			if path == "" {
				continue
			}
			info, err := os.Lstat(path)
			if err != nil {
				return fmt.Errorf("inspect restored output %s: %w", path, err)
			}
			if !filepath.IsAbs(path) || info.Mode()&os.ModeNamedPipe == 0 || seen[path] {
				return fmt.Errorf("restored output %s must identify a distinct FIFO", path)
			}
			seen[path] = true
		}
	}

	done := make(chan struct{})
	vm.RestoredIODone = done
	defer func() {
		attachments := append([]cio.IO(nil), vm.RestoredIOAttachments...)
		go func() {
			defer close(done)
			for _, attachment := range attachments {
				attachment.Wait()
			}
		}()
	}()
	for _, process := range streams {
		vm.RestoredIO = append(vm.RestoredIO, googleproto.Clone(process).(*proto.RestoredProcessIO))
		attachment, err := cio.NewAttach(cio.WithStreams(nil, stdout, stderr))(cio.NewFIFOSet(cio.Config{
			Stdout: process.Stdout, Stderr: process.Stderr, Terminal: process.Terminal,
		}, func() error { return nil }))
		if err != nil {
			return fmt.Errorf("attach restored process %s/%s output: %w", process.ID, process.ExecID, err)
		}
		vm.RestoredIOAttachments = append(vm.RestoredIOAttachments, attachment)
	}
	return nil
}

// closeRestoredOutput runs after StopVM has closed the runtime's FIFO writers.
// A deadline reports incomplete draining while keeping the log readers alive.
func closeRestoredOutput(ctx context.Context, vm *misc.VM) error {
	if vm.RestoredIODone != nil {
		select {
		case <-vm.RestoredIODone:
		case <-ctx.Done():
			return fmt.Errorf("wait for restored output EOF: %w", ctx.Err())
		}
	}
	var result error
	for _, attachment := range vm.RestoredIOAttachments {
		result = errors.Join(result, attachment.Close())
	}
	return result
}
