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

	"chained_function_eventing/eventschemas"
)

func callback(_ context.Context, event cloudevents.Event) (*cloudevents.Event, cloudevents.Result) {
	var body eventschemas.GreetingEventBody
	if err := event.DataAs(&body); err != nil {
		log.Fatalf("failed to extract CloudEvent data: %s", err)
	}

	log.Printf("received a GreetingEvent: name=`%s`", body.Name)

	response := cloudevents.NewEvent("1.0")
	response.SetID(event.ID())
	response.SetType("greeting")
	response.SetSource("consumer")
	response.SetExtension("vhivemetadata", event.Extensions()["vhivemetadata"])
	return &response, nil
}

func main() {
	log.SetPrefix("Consumer: ")
	log.SetFlags(log.Lmicroseconds | log.LUTC)
	log.Println("started")

	ceClient, err := cloudevents.NewClientHTTP()
	if err != nil {
		log.Fatalf("failed to create a CloudEvents client, %s", err)
	}

	if err := ceClient.StartReceiver(context.Background(), callback); err != nil {
		log.Fatalf("failed to start the receiver: %s", err)
	}
}
