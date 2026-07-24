package ctriface

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/containerd/containerd/namespaces"
	fcclient "github.com/firecracker-microvm/firecracker-containerd/firecracker-control/client"
	"github.com/firecracker-microvm/firecracker-containerd/proto"
	log "github.com/sirupsen/logrus"
)

// ShimPool keeps firecracker-containerd shims (and their VM networks) ready
// for use. Acquiring a shim therefore avoids the instance-creation path.
type ShimPool struct {
	mu        sync.Mutex
	available []string
	fcClient  *fcclient.Client
	poolSize  int
	counter   uint64
	logger    *log.Entry
	config    func(string) *proto.CreateVMRequest
	prepare   func(string) error
	namespace func(string) string
}

func NewShimPool(
	fcClient *fcclient.Client,
	poolSize int,
	config func(string) *proto.CreateVMRequest,
	prepare func(string) error,
	namespace func(string) string,
) *ShimPool {
	return &ShimPool{
		fcClient:  fcClient,
		poolSize:  poolSize,
		logger:    log.WithField("component", "shim-pool"),
		config:    config,
		prepare:   prepare,
		namespace: namespace,
	}
}

func (p *ShimPool) context(ctx context.Context, vmID string) context.Context {
	namespace := vmID
	if p.namespace != nil {
		namespace = p.namespace(vmID)
	}
	return namespaces.WithNamespace(ctx, namespace)
}

func (p *ShimPool) nextID() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.counter++
	return fmt.Sprintf("shim-%d-%d", os.Getpid(), p.counter)
}

func (p *ShimPool) create(ctx context.Context, vmID string) error {
	if p.prepare != nil {
		if err := p.prepare(vmID); err != nil {
			return fmt.Errorf("prepare network for %s: %w", vmID, err)
		}
	}
	req := &proto.PrepareShimRequest{VMID: vmID}
	if p.config != nil {
		req.CreateVmRequest = p.config(vmID)
	}
	_, err := p.fcClient.PrepareShim(p.context(ctx, vmID), req)
	return err
}

func (p *ShimPool) refill(ctx context.Context) {
	p.mu.Lock()
	needed := p.poolSize - len(p.available)
	p.mu.Unlock()
	for i := 0; i < needed; i++ {
		vmID := p.nextID()
		if err := p.create(ctx, vmID); err != nil {
			p.logger.WithError(err).WithField("vmID", vmID).Error("failed to pre-create shim")
			continue
		}
		p.mu.Lock()
		p.available = append(p.available, vmID)
		p.mu.Unlock()
	}
}

func (p *ShimPool) Initialize(ctx context.Context) { p.refill(ctx) }

func (p *ShimPool) Acquire(ctx context.Context) (string, error) {
	p.mu.Lock()
	if len(p.available) > 0 {
		vmID := p.available[0]
		p.available = p.available[1:]
		p.mu.Unlock()
		go p.refill(context.Background())
		return vmID, nil
	}
	p.mu.Unlock()

	vmID := p.nextID()
	if err := p.create(ctx, vmID); err != nil {
		return "", err
	}
	go p.refill(context.Background())
	return vmID, nil
}

func (p *ShimPool) Cleanup(ctx context.Context) error {
	p.mu.Lock()
	available := p.available
	p.available = nil
	p.mu.Unlock()
	var first error
	for _, vmID := range available {
		if _, err := p.fcClient.RemoveShim(p.context(ctx, vmID), &proto.RemoveShimRequest{VMID: vmID}); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// Discard removes a checked-out shim after a launch failure. Used shims are
// not returned to the pool because a task may have changed their state.
func (p *ShimPool) Discard(ctx context.Context, vmID string) error {
	_, err := p.fcClient.RemoveShim(p.context(ctx, vmID), &proto.RemoveShimRequest{VMID: vmID})
	return err
}

func (p *ShimPool) Available() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.available)
}
