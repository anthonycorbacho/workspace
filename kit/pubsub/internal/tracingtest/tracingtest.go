// Package tracingtest provides helpers to test the trace propagation of the pubsub backends.
package tracingtest

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// UsePropagator sets the W3C trace context propagator as global propagator, as kit/telemetry
// does, the previous propagator is restored once the test and its subtests completed.
func UsePropagator(t testing.TB) {
	t.Helper()
	prev := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(prev) })
}

// Recorder is a span processor recording the ended spans of its tracer.
type Recorder struct {
	tp    *sdktrace.TracerProvider
	mu    sync.Mutex
	ended []sdktrace.ReadOnlySpan
}

// NewRecorder returns a Recorder and sets the global propagator (see UsePropagator).
func NewRecorder(t testing.TB) *Recorder {
	t.Helper()
	UsePropagator(t)
	r := &Recorder{}
	r.tp = sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(r))
	t.Cleanup(func() { assert.NoError(t, r.tp.Shutdown(context.Background())) })
	return r
}

// Tracer returns a tracer whose spans are recorded.
func (r *Recorder) Tracer() trace.Tracer {
	return r.tp.Tracer("tracingtest")
}

// Ended returns the ended spans in the order they ended.
func (r *Recorder) Ended() []sdktrace.ReadOnlySpan {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]sdktrace.ReadOnlySpan(nil), r.ended...)
}

// OnStart implements sdktrace.SpanProcessor.
func (r *Recorder) OnStart(context.Context, sdktrace.ReadWriteSpan) {}

// OnEnd implements sdktrace.SpanProcessor.
func (r *Recorder) OnEnd(s sdktrace.ReadOnlySpan) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ended = append(r.ended, s)
}

// Shutdown implements sdktrace.SpanProcessor.
func (r *Recorder) Shutdown(context.Context) error { return nil }

// ForceFlush implements sdktrace.SpanProcessor.
func (r *Recorder) ForceFlush(context.Context) error { return nil }

// AssertPropagated asserts the recorded spans are a producer span named "publish <destination>"
// followed by a consumer span named "process <destination>", child of the producer span.
// processSpan is the consumer span returned to the backend.
func (r *Recorder) AssertPropagated(t testing.TB, destination string, processSpan trace.Span) {
	t.Helper()
	spans := r.Ended()
	require.Len(t, spans, 2)
	publish, process := spans[0], spans[1]

	assert.Equal(t, "publish "+destination, publish.Name())
	assert.Equal(t, trace.SpanKindProducer, publish.SpanKind())
	assert.Equal(t, "process "+destination, process.Name())
	assert.Equal(t, trace.SpanKindConsumer, process.SpanKind())

	assert.True(t, publish.SpanContext().IsValid())
	assert.Equal(t, publish.SpanContext().TraceID(), process.SpanContext().TraceID(), "the trace is continued")
	assert.Equal(t, publish.SpanContext().SpanID(), process.Parent().SpanID(), "process is a child of publish")
	assert.True(t, process.Parent().IsRemote())
	assert.Equal(t, process.SpanContext(), processSpan.SpanContext())
}
