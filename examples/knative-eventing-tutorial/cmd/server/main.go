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

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/client"
	"github.com/kelseyhightower/envconfig"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"pipeline/cmd"
)

// The struct for parsing the environment variable `K_SINK` that points
// to the URL of the downstream sink we will send our CloudEvents to.
// This environment variable set by SinkBinding, see
// https://knative.dev/docs/eventing/sources/sinkbinding/
type envConfig struct {
	// Sink URL where to send CloudEvents
	Sink string `envconfig:"K_SINK" required:"true"`
}

// Our server class, which does not contain any fields.
type server struct{}

// CloudEvent (ce) client for sending CloudEvents:
var ceClient client.Client

func main() {
	log.Println("Server started")

	var env envConfig
	if err := envconfig.Process("", &env); err != nil {
		log.Fatalf("Failed to process env var: %s", err)
	}
	log.Printf("K_SINK=%s", env.Sink)

	var err error
	ceProtocol, err := cloudevents.NewHTTP(cloudevents.WithTarget(env.Sink))
	if err != nil {
		log.Fatalf("Failed to create CloudEvent protocol: %s", err)
	}

	ceClient, err = cloudevents.NewClient(ceProtocol, cloudevents.WithUUIDs(), cloudevents.WithTimeNow())
	if err != nil {
		log.Fatalf("Failed to create CloudEvent client: %s", err)
	}

	lis, err := net.Listen("tcp", "0.0.0.0:8080")
	if err != nil {
		log.Fatalf("Failed to listen: %s", err)
	}
	defer lis.Close()

	var server server
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)
	RegisterTempReaderServer(grpcServer, &server)
	// To be able to use generic gRPC clients, such as grpcurl, it is cruical
	// to enable reflection support, that is, dynamic introspection of gRPC
	// services (i.e. available methods and their signatures) at runtime.
	reflection.Register(grpcServer)
	err = grpcServer.Serve(lis)
	if err != nil {
		log.Fatalf("Failed to serve: %s", err)
	}
}

func (s *server) ReadTemp(ctx context.Context, t *TempReading) (*Response, error) {
	log.Printf("read temperature: %d °C", t.Celsius)

	// Here, as an example, we do some validation. Any temperature reading
	// not between 0 and 100 are discaarded as being too cold or too hot
	// to be reasonable.
	// A real-world implementation might create an "outlier" event to detect
	// fault sensors for instance.
	if t.Celsius > 100 {
		return &Response{Ok: false, Reason: "too hot, cannot be more than 100 degrees"}, nil
	} else if t.Celsius < 0 {
		return &Response{Ok: false, Reason: "too col, cannot be less than 0 degrees"}, nil
	}

	// Creating the corresponding CloudEvent:
	//   - CloudEvent version is always 1.0
	//   - Source and type denote the subject and the action that a CloudEvent
	//     is created for. In our example, source/subject is a node (e.g.
	//     dedicated server) in a datacenter, and the type/action is
	//     temperature reading.
	//   - Both source and type are attributes that Knative triggers can
	//     filter on. In addition, we define provider and providerZone to let
	//     downstream users filter events by the cloud provider (e.g. AWS)
	//     and/or by the datacenter zone (e.g. us-east-1).
	event := cloudevents.NewEvent("1.0")
	event.SetType("temp")
	event.SetSource(fmt.Sprintf(
		"//node/%s/%s/%s", t.Provider, t.Zone, t.Machine,
	))
	event.SetExtension("provider",
		fmt.Sprintf("//provider/%s", t.Provider))
	event.SetExtension("providerZone",
		fmt.Sprintf("//provider-zone/%s/%s", t.Provider, t.Zone))

	if err := event.SetData(cloudevents.ApplicationJSON, cmd.TempEvent{Celsius: t.Celsius}); err != nil {
		log.Printf("Failed to set CloudEvents data: %s", err)
	}

	if res := ceClient.Send(ctx, event); !cloudevents.IsACK(res) {
		log.Printf("Failed to send CloudEvent: %+v", res)
	}

	return &Response{Ok: true}, nil
}

// Not sure what this stands for, presumably called upon unknown methods.
// From gRPC generated code:
// > All implementations must embed UnimplementedTempReaderServer
// > for forward compatibility
func (s *server) mustEmbedUnimplementedTempReaderServer() {
	panic("not implemented")
}
