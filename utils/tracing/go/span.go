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

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// Span is an abstraction for making multiple custom OpenTelemetry spans within a single
// component (e.g., within processing a single request by a server). Spans can be used to
// indicate what a process is working on as time progresses, and can be given a name to
// distinguish them. Spans are used in a hierarchy, whereby one "parent" span can have many
// "child" spans, and typially the parent span will be "running" for the duration of all
// its children spans (or potentially longer). This hierarchy is visible within zipkin.
type Span struct {
	SpanName, TracerName string
	traceSpan            *trace.Span
}

// StartSpan starts the span. A context, ctx, is used here to determine any parent spans, IDs, etc.
// A context is also returned, and it should be given as input to other spans if they should be
// marked as the chidlren of this span. This is how a parent / child hierarchy can be built between
// multiple spans in your functions.
func (s *Span) StartSpan(ctx context.Context) context.Context {
	tracer := otel.GetTracerProvider().Tracer(s.TracerName)
	newctx, traceSpan := tracer.Start(ctx, s.SpanName)
	s.traceSpan = &traceSpan
	return newctx
}

// EndSpan marks the span as finished at the time when this function is called.
func (s *Span) EndSpan() {
	(*s.traceSpan).End()
}
