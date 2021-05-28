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

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"

	"pipeline/cmd"
)

func callback(_ context.Context, event cloudevents.Event) (*cloudevents.Event, cloudevents.Result) {
	// Extract the body of the incoming CloudEvent into our struct:
	var tempEvent cmd.TempEvent
	if err := event.DataAs(&tempEvent); err != nil {
		log.Printf("Error while extracting CloudEvent data: %s", err)
		return nil, cloudevents.NewHTTPResult(400, "failed to extract data: %s", err)
	}

	if tempEvent.Celsius <= 80 {
		log.Printf("temp-consumer found that %s was %d °C at %s", event.Source(), tempEvent.Celsius, event.Time())
		return nil, nil
	}

	log.Printf("temp-consumer found that %s OVERHEATED to %d °C at %s", event.Source(), tempEvent.Celsius, event.Time())

	// Respond with type=overheat event:
	// All other attributes (source, provider, providerZone) remain the same.
	//
	// Responding with another event is optional and is intended to show how
	// to respond back with another event after processing.
	//
	// The response will go back into the Knative eventing system just like
	// any other event.

	newEvent := cloudevents.NewEvent()
	// Assign a new random ID. We must ensure that (`source`, `id`) is unique
	// for each event.
	newEvent.SetID(uuid.New().String())
	// Set type to `overheat`:
	newEvent.SetType("overheat")
	// Copy all the other attributes as-is:
	newEvent.SetSource(event.Source())
	newEvent.SetExtension("provider", event.Extensions()["provider"])
	newEvent.SetExtension("providerZone", event.Extensions()["providerZone"])

	if err := newEvent.SetData(cloudevents.ApplicationJSON, cmd.OverheatEvent{Celsius: tempEvent.Celsius}); err != nil {
		return nil, cloudevents.NewHTTPResult(500, "failed to set response data: %s", err)
	}
	return &newEvent, nil
}

func main() {
	log.Print("Temp-Consumer started")

	ceClient, err := cloudevents.NewClientHTTP()
	if err != nil {
		log.Fatalf("Failed to create client, %s", err)
	}

	if err := ceClient.StartReceiver(context.Background(), callback); err != nil {
		log.Fatalf("Receiver failed: %s", err)
	}
}
