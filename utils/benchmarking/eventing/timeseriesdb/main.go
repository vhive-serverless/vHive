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
	"log"
	"net"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ease-lab/vhive/utils/benchmarking/eventing/proto"
)

type Server struct {
	proto.UnimplementedTimeseriesServer

	completionEventDescriptorsT []*proto.CompletionEventDescriptor
	completionEventDescriptors  map[string][]*proto.CompletionEventDescriptor
	invocations                 map[string]*proto.InvocationDescriptor
}

func (s *Server) StartExperiment(_ context.Context, definition *proto.ExperimentDefinition) (*empty.Empty, error) {
	log.Printf("starting experiment %+v\n", definition.CompletionEventDescriptors)

	s.completionEventDescriptorsT = make([]*proto.CompletionEventDescriptor, len(definition.CompletionEventDescriptors))
	copy(s.completionEventDescriptorsT, definition.CompletionEventDescriptors)
	s.completionEventDescriptors = make(map[string][]*proto.CompletionEventDescriptor)
	s.invocations = make(map[string]*proto.InvocationDescriptor)

	return &empty.Empty{}, nil
}

func (s *Server) EndExperiment(_ context.Context, _ *empty.Empty) (*proto.ExperimentResult, error) {
	log.Println("ending current experiment")
	invocations := make([]*proto.InvocationDescriptor, 0)
	for _, inv := range s.invocations {
		invocations = append(invocations, inv)
	}
	return &proto.ExperimentResult{Invocations: invocations}, nil
}

func (s *Server) RegisterEvent(_ context.Context, event *proto.Event) (*empty.Empty, error) {
	now := time.Now().UTC()
	invocationId := event.VHiveMetadata.Id

	log.Printf(
		"registering event invocationId=`%s` id=`%s` source=`%s` type=`%s`\n",
		invocationId, event.Attributes["id"], event.Attributes["source"], event.Attributes["type"],
	)

	var invocation *proto.InvocationDescriptor
	var ok bool
	invocation, ok = s.invocations[invocationId]
	if !ok {
		s.completionEventDescriptors[invocationId] = make([]*proto.CompletionEventDescriptor, len(s.completionEventDescriptorsT))
		copy(s.completionEventDescriptors[invocationId], s.completionEventDescriptorsT)
		invocation = &proto.InvocationDescriptor{
			Id:           event.VHiveMetadata.Id,
			InvokedOn:    event.VHiveMetadata.InvokedOn,
			Duration:     nil,
			EventRecords: make([]*proto.EventRecord, 0),
			Status:       proto.InvocationStatus_CANCELLED,
		}
		s.invocations[invocationId] = invocation
	}

	isCompletion := false
	for i := 0; i < len(s.completionEventDescriptors[invocationId]); i++ {
		isCompletion = true
		for attr, expected := range s.completionEventDescriptors[invocationId][i].AttrMatchers {
			if actual, ok := event.Attributes[attr]; !ok {
				isCompletion = false
				break
			} else if actual != expected {
				isCompletion = false
				break
			}
		}
		if isCompletion {
			// Remove the matched element
			s.completionEventDescriptors[invocationId] =
				append(s.completionEventDescriptors[invocationId][:i], s.completionEventDescriptors[invocationId][i+1:]...)
		}
	}

	if isCompletion {
		sinceInvocation := now.Sub(event.VHiveMetadata.InvokedOn.AsTime())
		if invocation.Duration == nil || sinceInvocation > invocation.Duration.AsDuration() {
			invocation.Duration = durationpb.New(sinceInvocation)
		}
	}

	if len(s.completionEventDescriptors[invocationId]) == 0 {
		invocation.Status = proto.InvocationStatus_COMPLETED
	}

	invocation.EventRecords = append(invocation.EventRecords, &proto.EventRecord{
		Event:        event,
		RecordedOn:   timestamppb.New(now),
		IsCompletion: isCompletion,
	})

	return &empty.Empty{}, nil
}

func main() {
	log.SetPrefix("TimeseriesDB: ")
	log.SetFlags(log.Lmicroseconds | log.LUTC)
	log.Println("started")

	lis, err := net.Listen("tcp", "0.0.0.0:8080")
	if err != nil {
		log.Fatalf("failed to listen: %s", err)
	}
	defer lis.Close()

	var server Server
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)
	proto.RegisterTimeseriesServer(grpcServer, &server)
	reflection.Register(grpcServer)
	err = grpcServer.Serve(lis)
	if err != nil {
		log.Fatalf("failed to serve: %s", err)
	}
}
