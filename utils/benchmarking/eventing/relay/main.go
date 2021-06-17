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
	"log"
	"strconv"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	ctrdlog "github.com/containerd/containerd/log"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ease-lab/vhive/utils/benchmarking/eventing/proto"
)

const DatabaseAddress = "10.96.0.84:80"

func UnmarshalVHiveMetadata(s string) *proto.VHiveMetadata {
	var j struct {
		Id        string `json:"Id"`
		InvokedOn string `json:"InvokedOn"`
	}
	if err := json.Unmarshal([]byte(s), &j); err != nil {
		log.Fatalf("failed to unmarshal vhivemetadata: %s", err)
	}
	invokedOnT, err := time.Parse(ctrdlog.RFC3339NanoFixed, j.InvokedOn)
	if err != nil {
		log.Fatalf("failed to parse InvokedOn of vhivemetadata: %s", err)
	}
	return &proto.VHiveMetadata{
		Id:        j.Id,
		InvokedOn: timestamppb.New(invokedOnT),
	}
}

func callback(ctx context.Context, event cloudevents.Event) (*cloudevents.Event, cloudevents.Result) {
	vHiveMetadataString, ok := event.Extensions()["vhivemetadata"].(string)
	if !ok {
		log.Fatalln("vhivemetadata is not of type string")
	}

	log.Printf(
		"received id=`%s` source=`%s` type=`%s` vhivemetadata=`%s` data=`%s`\n",
		event.ID(), event.Source(), event.Type(), vHiveMetadataString, string(event.Data()),
	)

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

	conn, err := grpc.Dial(DatabaseAddress, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("failed to dial: %s", err)
	}
	defer func(conn *grpc.ClientConn) {
		err := conn.Close()
		if err != nil {
			log.Fatalf("failed to close: %s", err)
		}
	}(conn)
	timeseriesClient := proto.NewTimeseriesClient(conn)
	_, err = timeseriesClient.RegisterEvent(ctx, &proto.Event{
		VHiveMetadata: UnmarshalVHiveMetadata(vHiveMetadataString),
		Attributes:    attributes,
		Data:          event.Data(),
	})
	if err != nil {
		log.Fatalf("failed to register event: %s", err)
	}

	return nil, nil
}

func main() {
	log.SetPrefix("EventRelay: ")
	log.SetFlags(log.Lmicroseconds | log.LUTC)
	log.Println("started")

	ceClient, err := cloudevents.NewClientHTTP()
	if err != nil {
		log.Fatalf("failed to create client: %s", err)
	}

	log.Println("receiving...")

	if err := ceClient.StartReceiver(context.Background(), callback); err != nil {
		log.Fatalf("receiver failed: %s", err)
	}
}
