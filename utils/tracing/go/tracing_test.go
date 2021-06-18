package tracing

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func TestInitBasicTracer(t *testing.T) {
	InitBasicTracer("http://localhost:9999", "test-tracer")
	tracer := otel.GetTracerProvider().Tracer("test-tracer")
	_, traceSpan := tracer.Start(context.Background(), "test-span")
	traceSpan.End()
}

func TestInitCustomTracer(t *testing.T) {
	logger := log.New(os.Stderr, "tracer-log", log.Ldate|log.Ltime|log.Llongfile)
	InitCustomTracer("http://localhost:9999", 1.0, logger, attribute.String("service.name", "custom tracer"))
	tracer := otel.GetTracerProvider().Tracer("test-tracer")
	_, traceSpan := tracer.Start(context.Background(), "test-span")
	traceSpan.End()
}

func TestSpan(t *testing.T) {
	InitBasicTracer("http://localhost:9999", "test-tracer")
	span := Span{SpanName: "test-span", TracerName: "test-tracer"}
	span.StartSpan(context.Background())
	<-time.After(1 * time.Second)
	span.EndSpan()
}
