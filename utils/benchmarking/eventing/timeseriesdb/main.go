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

	workflows map[string]*Workflow
}

type Workflow struct {
	id                          string
	completionEventDescriptorsT []*proto.CompletionEventDescriptor
	invocations                 map[string]*Invocation
}

type Invocation struct {
	completionEventDescriptors []*proto.CompletionEventDescriptor
	descriptor                 *proto.InvocationDescriptor
}

func (s *Server) StartExperiment(_ context.Context, definition *proto.ExperimentDefinition) (*empty.Empty, error) {
	log.Infoln("starting experiment")

	s.workflows = make(map[string]*Workflow)
	for _, wd := range definition.WorkflowDefinitions {
		s.workflows[wd.Id] = &Workflow{
			id:                          wd.Id,
			completionEventDescriptorsT: make([]*proto.CompletionEventDescriptor, len(wd.CompletionEventDescriptors)),
			invocations:                 make(map[string]*Invocation),
		}
		copy(s.workflows[wd.Id].completionEventDescriptorsT, wd.CompletionEventDescriptors)
	}

	return &empty.Empty{}, nil
}

func (s *Server) EndExperiment(_ context.Context, _ *empty.Empty) (*proto.ExperimentResult, error) {
	log.Infoln("ending current experiment")
	results := make(map[string]*proto.WorkflowResult)
	for _, wrk := range s.workflows {
		invocations := make([]*proto.InvocationDescriptor, 0)
		for _, inv := range wrk.invocations {
			invocations = append(invocations, inv.descriptor)
		}
		results[wrk.id] = &proto.WorkflowResult{Invocations: invocations}

	}
	return &proto.ExperimentResult{WorkflowResults: results}, nil
}

func getProtoEventFromCloudEvent(s string) *proto.VHiveMetadata {
	var j struct {
		WorkflowId   string `json:"WorkflowId"`
		InvocationId string `json:"InvocationId"`
		InvokedOn    string `json:"InvokedOn"`
	}
	if err := json.Unmarshal([]byte(s), &j); err != nil {
		log.Fatalln("failed to unmarshal vhivemetadata", err)
	}
	invokedOnT, err := time.Parse(ctrdlog.RFC3339NanoFixed, j.InvokedOn)
	if err != nil {
		log.Fatalln("failed to parse InvokedOn of vhivemetadata", err)
	}
	return &proto.VHiveMetadata{
		WorkflowId:   j.WorkflowId,
		InvocationId: j.InvocationId,
		InvokedOn:    timestamppb.New(invokedOnT),
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
		VHiveMetadata: getProtoEventFromCloudEvent(vHiveMetadataString),
		Attributes:    attributes,
		Data:          event.Data(),
	}
}

func (s *Server) checkCompletion(event *proto.Event) (int, bool) {
	invocation := s.workflows[event.VHiveMetadata.WorkflowId].invocations[event.VHiveMetadata.InvocationId]
	for i := 0; i < len(invocation.completionEventDescriptors); i++ {
		isCompletion := true
		for attr, expected := range invocation.completionEventDescriptors[i].AttrMatchers {
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

func (s *Server) registerInvocation(vHiveMetadata *proto.VHiveMetadata) *Invocation {
	workflow := s.workflows[vHiveMetadata.WorkflowId]
	invocation := Invocation{
		completionEventDescriptors: make([]*proto.CompletionEventDescriptor, len(workflow.completionEventDescriptorsT)),
		descriptor: &proto.InvocationDescriptor{
			Id:           vHiveMetadata.InvocationId,
			InvokedOn:    vHiveMetadata.InvokedOn,
			Duration:     nil,
			EventRecords: make([]*proto.EventRecord, 0),
			Status:       proto.InvocationStatus_CANCELLED,
		},
	}
	copy(invocation.completionEventDescriptors, workflow.completionEventDescriptorsT)
	workflow.invocations[vHiveMetadata.InvocationId] = &invocation
	return &invocation
}

func (s *Server) onCompletionEvent(invDesc *proto.InvocationDescriptor, event *proto.Event, descriptorIdx int, now time.Time) {
	workflow := s.workflows[event.VHiveMetadata.WorkflowId]
	invocation := workflow.invocations[event.VHiveMetadata.InvocationId]

	// Remove the matched event descriptor
	invocation.completionEventDescriptors =
		append(invocation.completionEventDescriptors[:descriptorIdx], invocation.completionEventDescriptors[descriptorIdx+1:]...)

	// Update invDesc duration
	sinceInvocation := now.Sub(event.VHiveMetadata.InvokedOn.AsTime())
	if invDesc.Duration == nil || sinceInvocation > invDesc.Duration.AsDuration() {
		invDesc.Duration = durationpb.New(sinceInvocation)
	}

	// Mark invDesc as COMPLETED if no more completion events are awaited for
	if len(invocation.completionEventDescriptors) == 0 {
		invDesc.Status = proto.InvocationStatus_COMPLETED
	}
}

func (s *Server) registerEvent(_ context.Context, cloudEvent cloudevents.Event) (*cloudevents.Event, cloudevents.Result) {
	now := time.Now().UTC()
	event := morphCloudEventToProtoEvent(cloudEvent)

	log.Infof(
		"registering event workflowId=`%s` invocationId=`%s` id=`%s` source=`%s` type=`%s`\n",
		event.VHiveMetadata.WorkflowId, event.VHiveMetadata.InvocationId, event.Attributes["id"],
		event.Attributes["source"], event.Attributes["type"],
	)

	workflow := s.workflows[event.VHiveMetadata.WorkflowId]

	invocation, ok := workflow.invocations[event.VHiveMetadata.InvocationId]
	if !ok {
		invocation = s.registerInvocation(event.VHiveMetadata)
	}

	descriptorIdx, isCompletion := s.checkCompletion(event)
	if isCompletion {
		s.onCompletionEvent(invocation.descriptor, event, descriptorIdx, now)
	}

	invocation.descriptor.EventRecords = append(invocation.descriptor.EventRecords, &proto.EventRecord{
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
	grpcServer := grpc.NewServer()
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
