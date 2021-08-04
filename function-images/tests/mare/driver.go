// Copyright (c) 2021 Mert Bora Alper and EASE Lab
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

package main

import (
	"context"
	"net"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	tracing "github.com/ease-lab/vhive/utils/tracing/go"

	"github.com/ease-lab/mare"
)

type server struct {
	UnimplementedGreeterServer
}

func main() {
	if tracing.IsTracingEnabled() {
		zipkinURL := GetenvDefault("ZIPKIN_URL", "http://zipkin.istio-system.svc.cluster.local:9411/api/v2/spans")
		shutdown, err := tracing.InitBasicTracer(zipkinURL, "driver")
		if err != nil {
			logrus.Fatal("Failed to initialize tracing", err)
		}
		defer shutdown()
	}

	port := os.Getenv("PORT")
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		logrus.Fatal("Failed to listen: ", err)
	}
	defer lis.Close()

	logrus.Infof("Listening on :%s", port)

	var server server
	var grpcServer *grpc.Server
	if tracing.IsTracingEnabled() {
		grpcServer = tracing.GetGRPCServerWithUnaryInterceptor()
	} else {
		grpcServer = grpc.NewServer()
	}
	RegisterGreeterServer(grpcServer, &server)
	reflection.Register(grpcServer)
	err = grpcServer.Serve(lis)
	if err != nil {
		logrus.Fatal("Failed to serve: ", err)
	}
}

func (s *server) SayHello(ctx context.Context, req *HelloRequest) (*HelloReply, error) {
	workerURL := GetenvDefault("WORKER_URL", "127.0.0.1:8080")

	_, locator := mare.Drive(
		ctx,
		workerURL,
		"S3",
		"S3",
		os.Getenv("INTER_HINT"),
		"S3",
		os.Getenv("OUTPUT_HINT"),
		5,
		strings.Split(os.Getenv("INPUT_LOCATORS"), " "),
	)

	return &HelloReply{Message: locator}, nil
}

func GetenvDefault(key, default_ string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return default_
}
