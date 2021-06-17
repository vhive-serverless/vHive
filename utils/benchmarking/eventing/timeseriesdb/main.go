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
	"encoding/json"
	"net"
	"strconv"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	ctrdlog "github.com/containerd/containerd/log"
	"github.com/golang/protobuf/ptypes/empty"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"eventing/proto"
)

type Server struct {
	proto.UnimplementedTimeseriesServer

	completionEventDescriptorsT []*proto.CompletionEventDescriptor
	completionEventDescriptors  map[string][]*proto.CompletionEventDescriptor
	invocations                 map[string]*proto.InvocationDescriptor
}

func (s *Server) StartExperiment(_ context.Context, definition *proto.ExperimentDefinition) (*empty.Empty, error) {
	log.Infoln("starting experiment", definition.CompletionEventDescriptors)

	s.completionEventDescriptorsT = make([]*proto.CompletionEventDescriptor, len(definition.CompletionEventDescriptors))
	copy(s.completionEventDescriptorsT, definition.CompletionEventDescriptors)
	s.completionEventDescriptors = make(map[string][]*proto.CompletionEventDescriptor)
	s.invocations = make(map[string]*proto.InvocationDescriptor)

	return &empty.Empty{}, nil
}

func (s *Server) EndExperiment(_ context.Context, _ *empty.Empty) (*proto.ExperimentResult, error) {
	log.Infoln("ending current experiment")
	invocations := make([]*proto.InvocationDescriptor, 0)
	for _, inv := range s.invocations {
		invocations = append(invocations, inv)
	}
	return &proto.ExperimentResult{Invocations: invocations}, nil
}

func UnmarshalVHiveMetadata(s string) *proto.VHiveMetadata {
	var j struct {
		Id        string `json:"Id"`
		InvokedOn string `json:"InvokedOn"`
	}
	if err := json.Unmarshal([]byte(s), &j); err != nil {
		log.Fatalln("failed to unmarshal vhivemetadata", err)
	}
	invokedOnT, err := time.Parse(ctrdlog.RFC3339NanoFixed, j.InvokedOn)
	if err != nil {
		log.Fatalln("failed to parse InvokedOn of vhivemetadata", err)
	}
	return &proto.VHiveMetadata{
		Id:        j.Id,
		InvokedOn: timestamppb.New(invokedOnT),
	}
}

func morphCloudEventToProtoEvent(event cloudevents.Event) *proto.Event {
	vHiveMetadataString, ok := event.Extensions()["vhivemetadata"].(string)
	if !ok {
		log.Fatalln("vhivemetadata is not of type string")
	}

	// See https://github.com/cloudevents/spec/blob/v1.0.1/spec.md#type-system
	attributes := make(map[string]string)
	attributes["id"] = event.ID()
	attributes["source"] = event.Source()
	attributes["type"] = event.Type()
	for k, i := range event.Extensions() {
		switch v := i.(type) {
		case bool:
			attributes[k] = strconv.FormatBool(v)
		case int32:
			attributes[k] = strconv.FormatInt(int64(v), 10)
		case string:
			attributes[k] = v
		default:
			log.Fatalf("extension `%s` has an unsupported type", k)
		}
	}

	// CloudEvent extension `vhivemetadata` is our own magic and thus shall be removed from attributes
	delete(attributes, "vhivemetadata")

	return &proto.Event{
		VHiveMetadata: UnmarshalVHiveMetadata(vHiveMetadataString),
		Attributes:    attributes,
		Data:          event.Data(),
	}
}

func (s *Server) checkCompletion(event *proto.Event) (int, bool) {
	invocationId := event.VHiveMetadata.Id
	for i := 0; i < len(s.completionEventDescriptors[invocationId]); i++ {
		isCompletion := true
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
			return i, true
		}
	}
	return -1, false
}

func (s *Server) registerInvocation(vHiveMetadata *proto.VHiveMetadata) *proto.InvocationDescriptor {
	s.completionEventDescriptors[vHiveMetadata.Id] = make([]*proto.CompletionEventDescriptor, len(s.completionEventDescriptorsT))
	copy(s.completionEventDescriptors[vHiveMetadata.Id], s.completionEventDescriptorsT)
	invocation := &proto.InvocationDescriptor{
		Id:           vHiveMetadata.Id,
		InvokedOn:    vHiveMetadata.InvokedOn,
		Duration:     nil,
		EventRecords: make([]*proto.EventRecord, 0),
		Status:       proto.InvocationStatus_CANCELLED,
	}
	s.invocations[vHiveMetadata.Id] = invocation
	return invocation
}

func (s *Server) onCompletionEvent(invocation *proto.InvocationDescriptor, event *proto.Event, descriptorIdx int, now time.Time) {
	// Remove the matched event descriptor
	s.completionEventDescriptors[event.VHiveMetadata.Id] =
		append(s.completionEventDescriptors[invocation.Id][:descriptorIdx], s.completionEventDescriptors[invocation.Id][descriptorIdx+1:]...)

	// Update invocation duration
	sinceInvocation := now.Sub(event.VHiveMetadata.InvokedOn.AsTime())
	if invocation.Duration == nil || sinceInvocation > invocation.Duration.AsDuration() {
		invocation.Duration = durationpb.New(sinceInvocation)
	}

	// Mark invocation as COMPLETED if no more completion events are awaited for
	if len(s.completionEventDescriptors[invocation.Id]) == 0 {
		invocation.Status = proto.InvocationStatus_COMPLETED
	}
}

func (s *Server) registerEvent(_ context.Context, cloudEvent cloudevents.Event) (*cloudevents.Event, cloudevents.Result) {
	now := time.Now().UTC()
	event := morphCloudEventToProtoEvent(cloudEvent)

	log.Infof(
		"registering event invocationId=`%s` id=`%s` source=`%s` type=`%s`\n",
		event.VHiveMetadata.Id, event.Attributes["id"], event.Attributes["source"], event.Attributes["type"],
	)

	invocation, ok := s.invocations[event.VHiveMetadata.Id]
	if !ok {
		invocation = s.registerInvocation(event.VHiveMetadata)
	}

	descriptorIdx, isCompletion := s.checkCompletion(event)
	if isCompletion {
		s.onCompletionEvent(invocation, event, descriptorIdx, now)
	}

	invocation.EventRecords = append(invocation.EventRecords, &proto.EventRecord{
		Event:        event,
		RecordedOn:   timestamppb.New(now),
		IsCompletion: isCompletion,
	})

	return nil, nil
}

func main() {
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	log.Infoln("started")

	// Start the gRPC Server
	lis, err := net.Listen("tcp", "0.0.0.0:9090")
	if err != nil {
		log.Fatalln("failed to listen", err)
	}
	defer lis.Close()

	var server Server
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)
	proto.RegisterTimeseriesServer(grpcServer, &server)
	reflection.Register(grpcServer)
	go func() {
		if err = grpcServer.Serve(lis); err != nil {
			log.Fatalln("failed to serve", err)
		}
	}()

	// Start the CloudEvents receiver
	ceClient, err := cloudevents.NewClientHTTP()
	if err != nil {
		log.Fatalln("failed to create CloudEvents client", err)
	}
	if err := ceClient.StartReceiver(context.Background(), server.registerEvent); err != nil {
		log.Fatalln("failed to start CloudEvents receiver", err)
	}

}
