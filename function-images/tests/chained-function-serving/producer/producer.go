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
	"net"
	"os"

	ctrdlog "github.com/containerd/containerd/log"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	pb_client "tests/chained-functions-serving/proto"

	pb "github.com/ease-lab/vhive/examples/protobuf/helloworld"

	tracing "github.com/ease-lab/vhive/utils/tracing/go"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
)

type producerServer struct {
	consumerAddr string
	consumerPort int
	pb.UnimplementedGreeterServer
}

func (ps *producerServer) SayHello(ctx context.Context, req *pb.HelloRequest) (*pb.HelloReply, error) {
	// establish a connection
	addr := fmt.Sprintf("%v:%v", ps.consumerAddr, ps.consumerPort)
	conn, err := grpc.Dial(addr, grpc.WithInsecure(), grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()))
	if err != nil {
		log.Fatalf("[producer] fail to dial: %s", err)
	}
	defer conn.Close()

	client := pb_client.NewProducerConsumerClient(conn)

	// send message
	ack, err := client.ConsumeString(ctx, &pb_client.ConsumeStringRequest{Value: "1"})
	if err != nil {
		log.Fatalf("[producer] client error in string consumption: %s", err)
	}
	log.Printf("[producer] (single) Ack: %v\n", ack.Value)
	return &pb.HelloReply{Message: "Success"}, err
}

func main() {
	flagAddress := flag.String("addr", "consumer.default.192.168.1.240.sslip.io", "Server IP address")
	flagClientPort := flag.Int("pc", 80, "Client Port")
	flagServerPort := flag.Int("ps", 80, "Server Port")
	url := flag.String("zipkin", "http://zipkin.istio-system.svc.cluster.local:9411/api/v2/spans", "zipkin url")
	flag.Parse()

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	log.SetOutput(os.Stdout)

	shutdown, err := tracing.InitBasicTracer(*url, "producer")
	if err != nil {
		log.Warn(err)
	}
	defer shutdown()

	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(otelgrpc.UnaryServerInterceptor()))

	reflection.Register(grpcServer)
	// err = grpcServer.Serve(lis)

	//client setup
	log.Printf("[producer] Client using address: %v\n", *flagAddress)

	s := producerServer{}
	s.consumerAddr = *flagAddress
	s.consumerPort = *flagClientPort
	pb.RegisterGreeterServer(grpcServer, &s)

	//server setup
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *flagServerPort))
	if err != nil {
		log.Fatalf("[producer] failed to listen: %v", err)
	}

	log.Println("[producer] Server Started")

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("[producer] failed to serve: %s", err)
	}

}
