package manager

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// RestoreMaterializer owns the memory input presented to a restore. It can
// create a file-backed eager input or retain a PageSource for UFFD lazy input.
type RestoreMaterializer struct{}

type RestoreInput struct {
	MemoryPath string
	PageSource PageSource
	release    []func() error
}

func (i *RestoreInput) Close() error {
	if i == nil {
		return nil
	}
	var result error
	for n := len(i.release) - 1; n >= 0; n-- {
		if err := i.release[n](); err != nil {
			result = errors.Join(result, err)
		}
	}
	i.release = nil
	return result
}

// MaterializeEager writes a complete memory file through write. The callback
// is storage-specific; the file lifecycle remains owned by the restore layer.
func (RestoreMaterializer) MaterializeEager(memoryPath string, write func(io.Writer) error) (*RestoreInput, error) {
	if memoryPath == "" {
		return nil, fmt.Errorf("memory path is required for eager restore")
	}
	if write == nil {
		return nil, fmt.Errorf("eager memory writer is required")
	}
	file, err := os.OpenFile(memoryPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("create eager memory file: %w", err)
	}
	if err := write(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close eager memory file: %w", err)
	}
	return &RestoreInput{MemoryPath: memoryPath}, nil
}

// MaterializeLazy retains source until the restore input is closed. Callers
// pass it to PageServer when configuring Firecracker's UFFD backend.
func (RestoreMaterializer) MaterializeLazy(source PageSource) (*RestoreInput, error) {
	if source == nil {
		return nil, fmt.Errorf("page source is required for lazy restore")
	}
	return &RestoreInput{PageSource: source, release: []func() error{source.Close}}, nil
}

// NewPageServer transfers a lazy input's source to a PageServer. After this
// call the server owns source shutdown; Close on the input becomes a no-op.
func (i *RestoreInput) NewPageServer() (*PageServer, error) {
	if i == nil || i.PageSource == nil {
		return nil, fmt.Errorf("lazy page source is required")
	}
	server, err := NewPageServer(i.PageSource)
	if err != nil {
		return nil, err
	}
	i.PageSource = nil
	i.release = nil
	return server, nil
}

var _ io.Closer = (*RestoreInput)(nil)
