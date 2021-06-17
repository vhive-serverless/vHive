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
	"reflect"
	"testing"
	"time"

	ctrdlog "github.com/containerd/containerd/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ease-lab/vhive/utils/benchmarking/eventing/proto"
)

const SleepDuration = 200 * time.Millisecond

func TestDefaultEndToEnd(t *testing.T) {
	var server Server

	exDef := proto.ExperimentDefinition{CompletionEventDescriptors: []*proto.CompletionEventDescriptor{
		{
			AttrMatchers: map[string]string{
				"type":   "greeting",
				"source": "consumer",
			},
		},
	}}

	_, err := server.StartExperiment(context.TODO(), &exDef)
	if err != nil {
		t.Error("StartExperiment", err)
	}

	aInvokedOn := time.Now().UTC()

	eventA1 := proto.Event{
		VHiveMetadata: &proto.VHiveMetadata{Id: "A", InvokedOn: timestamppb.New(aInvokedOn)},
		Attributes: map[string]string{
			"id": "A",
			"source": "producer",
			"type": "greeting",
		},
		Data: []byte("some JSON data..."),
	}
	_, err = server.RegisterEvent(context.TODO(), &eventA1)
	if err != nil {
		t.Error("RegisterEvent(A1)", err)
	}

	time.Sleep(SleepDuration)

	eventA2 := proto.Event{
		VHiveMetadata: &proto.VHiveMetadata{Id: "A", InvokedOn: timestamppb.New(aInvokedOn)},
		Attributes: map[string]string{
			"id": "A",
			"source": "consumer",
			"type": "greeting",
		},
		Data: []byte("some JSON data..."),
	}
	_, err = server.RegisterEvent(context.TODO(), &eventA2)
	if err != nil {
		t.Error("RegisterEvent(A2)", err)
	}

	res, err := server.EndExperiment(context.TODO(), nil)
	if err != nil {
		t.Error("EndExperiment", err)
	}

	if len(res.Invocations) != 1 {
		t.Errorf("wrong length of Invocations: 1 != %d\n", len(res.Invocations))
	}

	invocation := res.Invocations[0]

	if invocation.Id != "A" {
		t.Errorf("wrong invocation Id: \"A\" != `%s`\n", invocation.Id)
	}

	if invocation.InvokedOn.AsTime() != aInvokedOn {
		t.Errorf("wrong invocation invokedOn: %s != %s\n", aInvokedOn.Format(ctrdlog.RFC3339NanoFixed), invocation.InvokedOn.AsTime().Format(ctrdlog.RFC3339NanoFixed))
	}

	if len(invocation.EventRecords) != 2 {
		t.Errorf("wrong length of invocation EventRecords: 2 != %d\n", len(invocation.EventRecords))
	}

	eventRecordA1 := invocation.EventRecords[0]

	if eventRecordA1.IsCompletion {
		t.Errorf("wrong invocation EventRecords[0] IsCompletion: false != %t\n", eventRecordA1.IsCompletion)
	}

	if !reflect.DeepEqual(eventRecordA1.Event, &eventA1) {
		t.Errorf("wrong invocation EventRecords[0] Event: %#v != %#v\n", &eventA1, eventRecordA1.Event)
	}

	eventRecordA2 := invocation.EventRecords[1]

	if !eventRecordA2.IsCompletion {
		t.Errorf("wrong invocation EventRecords[1] IsCompletion: true != %t\n", eventRecordA2.IsCompletion)
	}

	if !reflect.DeepEqual(eventRecordA2.Event, &eventA2) {
		t.Errorf("wrong invocation EventRecords[1] Event: %#v != %#v\n", &eventA2, eventRecordA2.Event)
	}

	if SleepDuration > invocation.Duration.AsDuration() {
		t.Errorf("wrong invocation Duration: %dms > %dms\n", SleepDuration.Milliseconds(), invocation.Duration.AsDuration().Milliseconds())
	}

	if invocation.Status != proto.InvocationStatus_COMPLETED {
		t.Errorf("wrong invocation Status: %s != %s\n", proto.InvocationStatus_COMPLETED.String(), invocation.Status.String())
	}
}
