package sandbox_pool

import (
	"context"
	"strconv"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/vhive/cri"
	criapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

const (
	poolSize = 10
)

type PodSandbox struct {
	ID string
}

// Service contains essential objects for host orchestration.
type PoolService struct {
	serv criapi.RuntimeServiceClient

	m    sync.Mutex
	pool []*PodSandbox
}

// NewService initializes the host orchestration state.
func NewPoolService() (*PoolService, error) {
	serv, err := cri.NewStockRuntimeServiceClient()

	s := &PoolService{
		serv: serv,
	}
	// pre-allocate the pool
	s.fillPool()

	return s, err
}

func (s *PoolService) CreateContainer(ctx context.Context, r *criapi.CreateContainerRequest) (*criapi.CreateContainerResponse, error) {
	return s.serv.CreateContainer(ctx, r)
}

func (s *PoolService) RemoveContainer(ctx context.Context, r *criapi.RemoveContainerRequest) (*criapi.RemoveContainerResponse, error) {
	return s.serv.RemoveContainer(ctx, r)
}

func (s *PoolService) RunPodSandbox(ctx context.Context, r *criapi.RunPodSandboxRequest) (*criapi.RunPodSandboxResponse, error) {
	// return s.serv.RunPodSandbox(ctx, r)
	return &criapi.RunPodSandboxResponse{
		PodSandboxId: s.pool[0].ID,
	}, nil
}

func (s *PoolService) fillPool() {
	s.m.Lock()
	defer s.m.Unlock()
	for i := 0; i < poolSize; i++ {
		resp, err := s.serv.RunPodSandbox(context.Background(), &criapi.RunPodSandboxRequest{
			Config: &criapi.PodSandboxConfig{
				Metadata: &criapi.PodSandboxMetadata{
					Name:      "sandbox-" + strconv.Itoa(i),
					Uid:       "sandbox-" + strconv.Itoa(i),
					Namespace: "sandbox",
				},
			},
		})

		if err != nil {
			log.Error(err)
			// handle error
		}

		s.pool = append(s.pool, &PodSandbox{ID: resp.PodSandboxId})
		// create a new PodSandbox
	}
}
