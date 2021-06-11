// Package tracing provides a simple utility for including opentelemetry and zipkin tracing
// instrumentation in vHive and Knative workloads
package tracing

import (
	"context"
	"fmt"
	"log"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/trace/zipkin"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv"
	"go.opentelemetry.io/otel/trace"
)

func initTracer(tp *sdktrace.TracerProvider) func() {
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return func() {
		_ = tp.Shutdown(context.Background())
	}
}

func newZipkinExporter(url string, logger *log.Logger) *zipkin.Exporter {
	exporter, err := zipkin.NewRawExporter(
		url,
		zipkin.WithLogger(logger),
		zipkin.WithSDKOptions(sdktrace.WithSampler(sdktrace.AlwaysSample())),
	)
	if err != nil {
		log.Fatal(err)
	}
	return exporter
}

// InitBasicTracer initialises a basic opentelemetry tracer using a zipkin exporter. The
// exporter exports to the provided url and the tracer takes on the given name concatenated
// with the host name (e.g. Knative pod name).
func InitBasicTracer(url string, serviceName string) func() {
	var logger = log.New(os.Stderr, "tracer-log", log.Ldate|log.Ltime|log.Llongfile)
	hostname, ok := os.LookupEnv("HOSTNAME")
	if !ok {
		hostname = ""
	}
	exporter := newZipkinExporter(url, logger)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSyncer(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.ServiceNameKey.String(fmt.Sprintf("%v@%v", serviceName, hostname)),
			attribute.Int64("ID", 1),
		)),
	)
	f := initTracer(tp)
	return f
}

// InitCustomTracer initialises an opentelemetry tracer. It uses a zipkin exporter which exports
// to the given url and logger, and samples traces at the specified traceRate. Any number of additional
// attributes (attr) can be provided.
func InitCustomTracer(url string, traceRate float64, logger *log.Logger, attr ...attribute.KeyValue) func() {
	exporter := newZipkinExporter(url, logger)
	var sampler sdktrace.Sampler
	if traceRate >= 1 {
		sampler = sdktrace.AlwaysSample()
	} else {
		sampler = sdktrace.ParentBased(sdktrace.TraceIDRatioBased(traceRate))
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sampler),
		sdktrace.WithSyncer(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			attr...,
		)),
	)
	f := initTracer(tp)
	return f
}

// Span gives a simple abstraction for making multiple opentelemetry spans within your process.
type Span struct {
	SpanName, TracerName string
	traceSpan            *trace.Span
}

// StartSpan starts the span. ctx is used to determine parent spans, IDs, etc.
// Returns a context which must be used in child spans such that they appear
// in the hierarchy.
func (s *Span) StartSpan(ctx context.Context) context.Context {
	tracer := otel.GetTracerProvider().Tracer(s.TracerName)
	newctx, traceSpan := tracer.Start(ctx, s.SpanName)
	s.traceSpan = &traceSpan
	return newctx
}

// EndSpan ends the span.
func (s *Span) EndSpan() {
	(*s.traceSpan).End()
}
