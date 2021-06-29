// MIT License
//
// Copyright (c) 2021 Michal Baczun and EASE lab
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
	"flag"
	"fmt"
	"os"
	"time"

	ctrdlog "github.com/containerd/containerd/log"
	pb "github.com/ease-lab/vhive/examples/protobuf/helloworld"

	tracing "github.com/ease-lab/vhive/utils/tracing/go"
	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

func main() {
	address := flag.String("addr", "localhost", "Server IP address")
	clientPort := flag.Int("pc", 50051, "Client Port")
	url := flag.String("zipkin", "http://zipkin.istio-system.svc.cluster.local:9411/api/v2/spans", "zipkin url")
	flag.Parse()

	fmt.Printf("Client using address: %v\n", *address)

	time.Sleep(10 * time.Second)

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	log.SetOutput(os.Stdout)

	shutdown, err := tracing.InitBasicTracer(*url, "go client")
	if err != nil {
		log.Warn(err)
	}
	defer shutdown()

	conn, err := grpc.Dial(fmt.Sprintf("%v:%v", *address, *clientPort), grpc.WithInsecure(), grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()))
	if err != nil {
		log.Fatalf("fail to dial: %s", err)
	}
	defer conn.Close()

	client := pb.NewGreeterClient(conn)
	empty, err := client.SayHello(context.Background(), &pb.HelloRequest{Name: "client"})
	fmt.Printf("Client output: %v, %v\n", empty, err)
	if err != nil {
		log.Fatalf("Error: %s", err)
	}
	fmt.Printf("client closing\n")

}
