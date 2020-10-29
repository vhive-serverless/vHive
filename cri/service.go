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

type CriService struct {
	criapi.ImageServiceServer
	criapi.RuntimeServiceServer
	orch               *ctriface.Orchestrator
	stockRuntimeClient criapi.RuntimeServiceClient
	stockImageClient   criapi.ImageServiceClient
}

func NewCriService(orch *ctriface.Orchestrator) (*CriService, error) {
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

	cs := &CriService{
		orch:               orch,
		stockRuntimeClient: stockRuntimeClient,
		stockImageClient:   stockImageClient,
	}

	return cs, nil
}

func (s *CriService) Register(server *grpc.Server) {
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
