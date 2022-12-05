// MIT License
//
// Copyright (c) 2020 Nathaniel Tornow and EASE lab
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
	"net"
	"time"

	"google.golang.org/grpc"
	criapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

const (
	stockCtrdSockAddr = "/run/firecracker-containerd/containerd.sock"
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
