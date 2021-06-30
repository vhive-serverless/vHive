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

// Package tracing provides a simple utility for including opentelemetry and zipkin tracing
// instrumentation in vHive and Knative workloads
package tracing

import (
	"context"
	"fmt"
	"log"
	"os"

	ctrdlog "github.com/containerd/containerd/log"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/trace/zipkin"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv"

	"google.golang.org/grpc"
)

func initTracer(tp *trace.TracerProvider) func() {
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return func() {
		err := tp.Shutdown(context.Background())
		if err != nil {
			log.Fatalf("Tracer shutdown error: %v", err)
		}
	}
}

func newZipkinExporter(url string, logger *log.Logger) (*zipkin.Exporter, error) {
	exporter, err := zipkin.NewRawExporter(
		url,
		zipkin.WithLogger(logger),
		zipkin.WithSDKOptions(trace.WithSampler(trace.AlwaysSample())),
	)
	if err != nil {
		log.Printf("warning: zipkin exporter experienced an error: %v", err)
	}
	return exporter, err
}

// InitBasicTracer initialises a basic OpenTelemetry tracer using a zipkin exporter. The
// exporter sends span and trace information to the provided URL. The tracer uses the name
// provided, concatenated with the name of the host (e.g., Knative pod name or docker container),
// and all nodes / functions which use this tracer will be labeled with "-FU" to denote that they
// are functions, which distinguishes them from knative queue proxies if used in a cluster.
func InitBasicTracer(url string, serviceName string) (func(), error) {
	logrus.SetFormatter(&logrus.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	var logger = log.New(os.Stderr, "tracer-log", log.Ldate|log.Ltime|log.Llongfile)
	hostname, ok := os.LookupEnv("HOSTNAME")
	if !ok {
		hostname = ""
	}
	exporter, err := newZipkinExporter(url, logger)
	if err != nil {
		return nil, err
	}
	tp := trace.NewTracerProvider(
		trace.WithSampler(trace.AlwaysSample()),
		trace.WithSyncer(exporter),
		trace.WithResource(resource.NewWithAttributes(
			semconv.ServiceNameKey.String(fmt.Sprintf("%v@%v-FU", serviceName, hostname)),
			attribute.Int64("ID", 1),
		)),
	)
	f := initTracer(tp)
	return f, nil
}

// InitCustomTracer initialises an OpenTelemetry tracer. It uses a zipkin exporter which exports
// to the given URL and logger, and samples traces at the specified traceRate. Any number of additional
// attributes (attr) can be provided. Note that this tracer uses the default name, which will be the name
// of the process according to the OS, so to name your tracer provide a "service.name" attribute.
func InitCustomTracer(url string, traceRate float64, logger *log.Logger, attr ...attribute.KeyValue) (func(), error) {
	exporter, err := newZipkinExporter(url, logger)
	if err != nil {
		return nil, err
	}
	var sampler trace.Sampler
	if traceRate >= 1 {
		sampler = trace.AlwaysSample()
	} else {
		sampler = trace.ParentBased(trace.TraceIDRatioBased(traceRate))
	}
	tp := trace.NewTracerProvider(
		trace.WithSampler(sampler),
		trace.WithSyncer(exporter),
		trace.WithResource(resource.NewWithAttributes(
			attr...,
		)),
	)
	f := initTracer(tp)
	return f, nil
}

// GetGRPCServerWithUnaryInterceptor returns a grpc server instrumented with an opentelemetry
// interceptor which enables tracing of grpc requests.
func GetGRPCServerWithUnaryInterceptor() *grpc.Server {
	return grpc.NewServer(grpc.UnaryInterceptor(otelgrpc.UnaryServerInterceptor()))
}

// DialGRPCWithUnaryInterceptor creates a connection to the provided address, which is instrumented
// with an opentelemetry client interceptor enabling the tracing to client grpc messages.
func DialGRPCWithUnaryInterceptor(addr string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	opts = append(opts, grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()))
	return grpc.Dial(addr, opts...)
}
