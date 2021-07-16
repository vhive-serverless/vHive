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
	"io"
	"net"
	"os"

	ctrdlog "github.com/containerd/containerd/log"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	pb "tests/chained-functions-serving/proto"

	tracing "github.com/ease-lab/vhive/utils/tracing/go"

	sdk "github.com/ease-lab/vhive-xdt/sdk/golang"
	"github.com/ease-lab/vhive-xdt/utils"
)

type consumerServer struct {
	pb.UnimplementedProducerConsumerServer
}

func (s *consumerServer) ConsumeString(ctx context.Context, str *pb.ConsumeStringRequest) (*pb.ConsumeStringReply, error) {
	if tracing.IsTracingEnabled() {
		span1 := tracing.Span{SpanName: "custom-span-1", TracerName: "tracer"}
		span2 := tracing.Span{SpanName: "custom-span-2", TracerName: "tracer"}
		ctx = span1.StartSpan(ctx)
		ctx = span2.StartSpan(ctx)
		defer span1.EndSpan()
		defer span2.EndSpan()
	}
	log.Printf("[consumer] Consumed %v\n", str.Value)
	return &pb.ConsumeStringReply{Value: true}, nil
}

func (s *consumerServer) ConsumeStream(stream pb.ProducerConsumer_ConsumeStreamServer) error {
	for {
		str, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&pb.ConsumeStringReply{Value: true})
		}
		if err != nil {
			return err
		}
		log.Printf("[consumer] Consumed string of length %d\n", len(str.Value))
	}
}

func main() {
	port := flag.Int("ps", 80, "Port")
	url := flag.String("zipkin", "http://zipkin.istio-system.svc.cluster.local:9411/api/v2/spans", "zipkin url")
	flag.Parse()

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	log.SetOutput(os.Stdout)

	if tracing.IsTracingEnabled() {
		log.Println("consumer has tracing enabled")
		shutdown, err := tracing.InitBasicTracer(*url, "consumer")
		if err != nil {
			log.Warn(err)
		}
		defer shutdown()
	} else {
		log.Println("consumer has tracing DISABLED")
	}

	transferType, ok := os.LookupEnv("TRANSFER_TYPE")
	if !ok {
		log.Infof("TRANSFER_TYPE not found, using INLINE transfer")
		transferType = "INLINE"
	}

	if transferType == "INLINE" {
		//set up server
		lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
		if err != nil {
			log.Fatalf("[consumer] failed to listen: %v", err)
		}

		var grpcServer *grpc.Server
		if tracing.IsTracingEnabled() {
			grpcServer = tracing.GetGRPCServerWithUnaryInterceptor()
		} else {
			grpcServer = grpc.NewServer()
		}

		s := consumerServer{}
		pb.RegisterProducerConsumerServer(grpcServer, &s)

		log.Printf("[consumer] Server Started on port %v\n", *port)

		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("[consumer] failed to serve: %s", err)
		}
	} else if transferType == "XDT" {
		var handler = func(data []byte) {
			log.Infof("gx: destination handler received data of size %d", len(data))
		}
		config := utils.ReadConfig()
		sdk.StartDstServer(config, handler)
	}
}
