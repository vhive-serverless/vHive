// MIT License
//
// Copyright (c) 2020 Plamen Petrov and EASE lab
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
	"github.com/containerd/containerd"
	"sync"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	criapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// Service contains essential objects for host orchestration.
type Service struct {
	sync.Mutex

	criapi.ImageServiceServer
	criapi.RuntimeServiceServer
	stockRuntimeClient criapi.RuntimeServiceClient
	stockImageClient   criapi.ImageServiceClient

	// generic coordinator
	serv ServiceInterface

	// stock containerd client (not cri interface)
	client *containerd.Client
}

// NewService initializes the host orchestration state.
func NewService(serv ServiceInterface) (*Service, error) {
	if serv == nil {
		return nil, errors.New("coor must be non nil")
	}

	stockRuntimeClient, err := NewStockRuntimeServiceClient()
	if err != nil {
		log.WithError(err).Error("failed to create new stock runtime service client")
		return nil, err
	}

	stockImageClient, err := NewStockImageServiceClient()
	if err != nil {
		log.WithError(err).Error("failed to create new stock image service client")
		return nil, err
	}

	client, err := containerd.New("/run/containerd/containerd.sock",
		containerd.WithDefaultNamespace("k8s.io"),
	)
	if err != nil {
		log.WithError(err).Error("Failed to create containerd client")
	}

	cs := &Service{
		stockRuntimeClient: stockRuntimeClient,
		stockImageClient:   stockImageClient,
		serv:               serv,
		client:             client,
	}

	return cs, nil
}

func (s *Service) CreateContainer(ctx context.Context, r *criapi.CreateContainerRequest) (*criapi.CreateContainerResponse, error) {
	response, err := s.serv.CreateContainer(ctx, r)

	//imageRef := r.GetConfig().GetImage().GetImage()
	//image, err := s.client.GetImage(ctx, imageRef)
	//if err != nil {
	//	log.WithError(err)
	//} else {
	//	log.Infof("Image %s present", image.Name())
	//}

	return response, err
}

func (s *Service) RemoveContainer(ctx context.Context, r *criapi.RemoveContainerRequest) (*criapi.RemoveContainerResponse, error) {
	return s.serv.RemoveContainer(ctx, r)
}

// Register registers the criapi servers.
func (s *Service) Register(server *grpc.Server) {
	criapi.RegisterImageServiceServer(server, s)
	criapi.RegisterRuntimeServiceServer(server, s)
}
