// MIT License
//
// Copyright (c) 2021 Michal Baczun and EASE lab
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
	"flag"
	"fmt"
	"log"
	"os"

	"google.golang.org/grpc"

	pb "github.com/ease-lab/vhive/examples/protobuf/helloworld"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/trace/zipkin"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv"
)

const (
	serviceName = "client"
	id          = 3
)

var logger = log.New(os.Stderr, "zipkin-example", log.Ldate|log.Ltime|log.Llongfile)

// initTracer creates a new trace provider instance and registers it as global trace provider.
func initTracer(url string) func() {
	// Create Zipkin Exporter and install it as a global tracer.
	//
	// For demoing purposes, always sample. In a production application, you should
	// configure the sampler to a trace.ParentBased(trace.TraceIDRatioBased) set at the desired
	// ratio.
	exporter, err := zipkin.NewRawExporter(
		url,
		zipkin.WithLogger(logger),
		zipkin.WithSDKOptions(sdktrace.WithSampler(sdktrace.AlwaysSample())),
	)
	if err != nil {
		log.Fatal(err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSyncer(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			attribute.Int64("ID", id),
		)),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return func() {
		_ = tp.Shutdown(context.Background())
	}
}

func main() {
	//strings := flag.Int("s", 1, "Number of strings to send")
	address := flag.String("addr", "localhost", "Server IP address")
	clientPort := flag.Int("pc", 3031, "Client Port")
	url := flag.String("zipkin", "http://localhost:9411/api/v2/spans", "zipkin url")
	flag.Parse()

	shutdown := initTracer(*url)
	defer shutdown()

	fmt.Printf("Client using address: %v\n", *address)

	conn, err := grpc.Dial(fmt.Sprintf("%v:%v", *address, *clientPort), grpc.WithInsecure(), grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()))
	if err != nil {
		log.Fatalf("fail to dial: %s", err)
	}
	defer conn.Close()

	client := pb.NewGreeterClient(conn)
	empty, err := client.SayHello(context.Background(), &pb.HelloRequest{Name: "client"})
	fmt.Printf("Client output: %v, %v\n", empty, err)
	if err != nil {
		log.Fatalf("Failed to produce strings: %s", err)
	}
	fmt.Printf("client closing\n")

}
