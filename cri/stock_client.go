package cri

import (
	"context"
	"net"
	"time"

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

func NewStockImageServiceClient() (criapi.ImageServiceClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, stockCtrdSockAddr, getDialOpts()...)
	if err != nil {
		return nil, err
	}

	return criapi.NewImageServiceClient(conn), nil
}

func NewStockRuntimeServiceClient() (criapi.RuntimeServiceClient, error) {
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
