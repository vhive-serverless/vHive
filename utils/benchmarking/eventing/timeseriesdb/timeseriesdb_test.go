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
	"net/url"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/types"
	ctrdlog "github.com/containerd/containerd/log"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"eventing/proto"
)

const SleepDuration = 200 * time.Millisecond

func parseURIRef(s string) types.URIRef {
	if u, err := url.Parse(s); err != nil {
		panic(err)
	} else {
		return types.URIRef{URL: *u}
	}
}

func TestDefaultEndToEnd(t *testing.T) {
	var server Server

	// Parameters
	// ================================================================
	exDef := proto.ExperimentDefinition{CompletionEventDescriptors: []*proto.CompletionEventDescriptor{
		{
			AttrMatchers: map[string]string{
				"type":   "greeting",
				"source": "consumer",
			},
		},
	}}
	aInvokedOn := time.Now().UTC()

	vHiveMetadataString := fmt.Sprintf(
		`{"Id": "A", "InvokedOn": "%s"}`,
		aInvokedOn.Format(ctrdlog.RFC3339NanoFixed),
	)

	cEventA1 := cloudevents.NewEvent("1.0")
	cEventA1.SetID("A")
	cEventA1.SetSource("producer")
	cEventA1.SetType("greeting")
	cEventA1.SetExtension("vhivemetadata", vHiveMetadataString)
	eventA1 := proto.Event{
		VHiveMetadata: &proto.VHiveMetadata{Id: "A", InvokedOn: timestamppb.New(aInvokedOn)},
		Attributes: map[string]string{
			"id":     "A",
			"source": "producer",
			"type":   "greeting",
		},
	}

	cEventA2 := cloudevents.NewEvent("1.0")
	cEventA2.SetID("A")
	cEventA2.SetSource("consumer")
	cEventA2.SetType("greeting")
	cEventA2.SetExtension("vhivemetadata", vHiveMetadataString)
	eventA2 := proto.Event{
		VHiveMetadata: &proto.VHiveMetadata{Id: "A", InvokedOn: timestamppb.New(aInvokedOn)},
		Attributes: map[string]string{
			"id":     "A",
			"source": "consumer",
			"type":   "greeting",
		},
	}

	// Scenario
	// ================================================================
	// invoker calls StartExperiment():
	if _, err := server.StartExperiment(context.Background(), &exDef); err != nil {
		t.Error("StartExperiment", err)
	}

	// trigger causes a call to registerEvent() upon the CloudEvent (A1) created by producer:
	if _, err := server.registerEvent(context.Background(), cEventA1); err != nil {
		t.Error("RegisterEvent(A1)", err)
	}

	// consumer works for a while:
	time.Sleep(SleepDuration)

	// trigger causes a call to registerEvent() upon the CloudEvent (A2) created the consumer:
	if _, err := server.registerEvent(context.Background(), cEventA2); err != nil {
		t.Error("RegisterEvent(A2)", err)
	}

	// invoker calls EndExperiment() to collect the results:
	res, err := server.EndExperiment(context.Background(), nil)
	if err != nil {
		t.Error("EndExperiment", err)
	}

	// Assertions
	// ================================================================
	require.Len(t, res.Invocations, 1)

	invocation := res.Invocations[0]
	require.Equal(t, "A", invocation.Id)
	require.Equal(t, aInvokedOn, invocation.InvokedOn.AsTime())
	require.GreaterOrEqual(t, invocation.Duration.AsDuration().Microseconds(), SleepDuration.Microseconds())
	require.Equal(t, proto.InvocationStatus_COMPLETED, invocation.Status)
	require.Len(t, invocation.EventRecords, 2)

	eventRecordA1 := invocation.EventRecords[0]
	require.False(t, eventRecordA1.IsCompletion)
	require.Equal(t, &eventA1, eventRecordA1.Event)

	eventRecordA2 := invocation.EventRecords[1]
	require.True(t, eventRecordA2.IsCompletion)
	require.Equal(t, &eventA2, eventRecordA2.Event)
}
