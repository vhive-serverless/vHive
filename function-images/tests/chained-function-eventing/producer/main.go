// MIT License
//
// Copyright (c) 2021 Mert Bora Alper and EASE lab
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
	"fmt"
	"log"
	"net"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/client"
	ctrdlog "github.com/containerd/containerd/log"
	"github.com/google/uuid"
	"github.com/kelseyhightower/envconfig"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"

	. "eventing/eventschemas"
)

type envConfig struct {
	// Sink URL where to send CloudEvents
	Sink string `envconfig:"K_SINK" required:"true"`
}

type server struct {
	UnimplementedGreeterServer
}

var ceClient client.Client

func (s *server) SayHello(ctx context.Context, req *HelloRequest) (*HelloReply, error) {
	id := uuid.New().String()

	if headers, ok := metadata.FromIncomingContext(ctx); ok {
		log.Printf("received an HelloRequest: name=`%s` (id=`%s` traceID=`%s`)", req.Name, id, headers.Get("x-b3-traceid")[0])
	} else {
		log.Printf("received an HelloRequest: name=`%s` (id=`%s`)", req.Name, id)
	}

	event := cloudevents.NewEvent("1.0")
	event.SetID(id)
	event.SetType("greeting")
	event.SetSource("producer")
	event.SetExtension(
		"vhivemetadata",
		fmt.Sprintf("{\"Id\": \"%s\", \"InvokedOn\": \"%s\"}", id, time.Now().UTC().Format(ctrdlog.RFC3339NanoFixed)),
	)

	if err := event.SetData(cloudevents.ApplicationJSON, GreetingEventBody{Name: req.Name}); err != nil {
		log.Fatalf("failed to set CloudEvents data: %s", err)
	}

	var env envConfig
	if err := envconfig.Process("", &env); err != nil {
		log.Fatalf("failed to process env var: %s", err)
	}

	// Send that Event.
	if result := ceClient.Send(cloudevents.ContextWithTarget(ctx, env.Sink), event); !cloudevents.IsACK(result) {
		log.Fatalf("failed to send CloudEvent: %+v", result)
	}

	log.Printf("responding to the client")
	return &HelloReply{Message: fmt.Sprintf("Hello, %s!", req.Name)}, nil
}

func main() {
	log.SetPrefix("Producer: ")
	log.SetFlags(log.Lmicroseconds | log.LUTC)
	log.Printf("started")

	var err error

	ceClient, err = opencensus.client.NewClientHTTP() // cloudevents.NewClientHTTP()
	if err != nil {
		log.Fatalf("failed to initialize CE client: %v", err)
	}

	lis, err := net.Listen("tcp", "0.0.0.0:8080")
	if err != nil {
		log.Fatalf("failed to listen: %s", err)
	}
	defer lis.Close()

	var server server
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)
	RegisterGreeterServer(grpcServer, &server)
	reflection.Register(grpcServer)
	err = grpcServer.Serve(lis)
	if err != nil {
		log.Fatalf("failed to serve: %s", err)
	}
}
