// MIT License
//
// Copyright (c) 2020 Plamen Petrov
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

package cri

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/ustiugov/fccd-orchestrator/ctriface"
	"google.golang.org/grpc"
	criapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

const (
	stockCtrdSockAddr = "/run/containerd/containerd.sock"
	dialTimeout       = 10 * time.Second
	// maxMsgSize use 16MB as the default message size limit.
	// grpc library default is 4MB
	maxMsgSize = 1024 * 1024 * 16
)

// Service contains essential objects for host orchestration.
type Service struct {
	sync.Mutex

	criapi.ImageServiceServer
	criapi.RuntimeServiceServer
	orch               *ctriface.Orchestrator
	stockRuntimeClient criapi.RuntimeServiceClient
	stockImageClient   criapi.ImageServiceClient
	coordinator        *coordinator

	// to store mapping from pod to guest image and port temporarily
	podVMConfigs map[string]*VMConfig
}

// VMConfig wraps the IP and port of the guest VM
type VMConfig struct {
	guestIP   string
	guestPort string
}

// NewService initializes the host orchestration state.
func NewService(orch *ctriface.Orchestrator) (*Service, error) {
	if orch == nil {
		return nil, errors.New("orch must be non nil")
	}

	stockRuntimeClient, err := newStockRuntimeServiceClient()
	if err != nil {
		log.WithError(err).Error("failed to create new stock runtime service client")
		return nil, err
	}

	stockImageClient, err := newStockImageServiceClient()
	if err != nil {
		log.WithError(err).Error("failed to create new stock image service client")
		return nil, err
	}

	cs := &Service{
		orch:               orch,
		stockRuntimeClient: stockRuntimeClient,
		stockImageClient:   stockImageClient,
		coordinator:        newCoordinator(orch),
		podVMConfigs:       make(map[string]*VMConfig),
	}

	return cs, nil
}

// Register registers the criapi servers.
func (s *Service) Register(server *grpc.Server) {
	criapi.RegisterImageServiceServer(server, s)
	criapi.RegisterRuntimeServiceServer(server, s)
}

func newStockImageServiceClient() (criapi.ImageServiceClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, stockCtrdSockAddr, getDialOpts()...)
	if err != nil {
		return nil, err
	}

	return criapi.NewImageServiceClient(conn), nil
}

func newStockRuntimeServiceClient() (criapi.RuntimeServiceClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, stockCtrdSockAddr, getDialOpts()...)
	if err != nil {
		return nil, err
	}

	return criapi.NewRuntimeServiceClient(conn), nil
}

func dialer(ctx context.Context, addr string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "unix", addr)
}

func getDialOpts() []grpc.DialOption {
	return []grpc.DialOption{
		grpc.WithInsecure(),
		grpc.WithContextDialer(dialer),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMsgSize)),
	}
}

func (s *Service) insertPodConfig(podID string, vmConfig *VMConfig) {
	s.Lock()
	defer s.Unlock()

	s.podVMConfigs[podID] = vmConfig
}

func (s *Service) removePodConfig(podID string) {
	s.Lock()
	defer s.Unlock()

	delete(s.podVMConfigs, podID)
}

func (s *Service) getPodConfig(podID string) (*VMConfig, error) {
	s.Lock()
	defer s.Unlock()

	vmConfig, isPresent := s.podVMConfigs[podID]
	if !isPresent {
		log.Errorf("VM config for pod %s does not exist", podID)
		return nil, errors.New("VM config for pod does not exist")
	}

	return vmConfig, nil
}
