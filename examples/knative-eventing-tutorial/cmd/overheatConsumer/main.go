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

	"pipeline/cmd"
)

func callback(_ context.Context, event cloudevents.Event) (*cloudevents.Event, cloudevents.Result) {
	var overheatEvent cmd.OverheatEvent

	// Extract the body of the incoming CloudEvent into our struct:
	if err := event.DataAs(&overheatEvent); err != nil {
		log.Printf("Error while extracting CloudEvent Data: %s", err)
		return nil, cloudevents.NewHTTPResult(400, "failed to convert data: %s", err)
	}

	// We do not do much in our toy example here. As suggested, a real-world
	// implementation could alert the on-site engineers or power off the
	// machines.
	log.Printf("overheat-consumer is alerting the engineers for %s (%d °C)", event.Source(), overheatEvent.Celsius)
	return nil, nil
}

func main() {
	log.Print("Overheat-Consumer started")

	ceClient, err := cloudevents.NewClientHTTP()
	if err != nil {
		log.Fatalf("Failed to create client, %v", err)
	}

	if err := ceClient.StartReceiver(context.Background(), callback); err != nil {
		log.Fatalf("Receiver failed: %s", err)
	}
}
