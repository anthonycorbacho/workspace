package telemetry

import (
	"context"
	"strconv"

	"github.com/anthonycorbacho/workspace/kit/config"
	"github.com/anthonycorbacho/workspace/kit/errors"
	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// NewTracer returns a new and configured TracerProvider.
//
// You can define the Trace sample rate by env var via OTL_TRACE_SAMPLE_RATE
// You can define the OTL endpoint by env var via OTL_ENDPOINT
// A list of attributes can be passed via env variable OTEL_RESOURCE_ATTRIBUTES;
//
// eg:
//
//	OTEL_RESOURCE_ATTRIBUTES=service.version=0.0.1,service.namespace=default
//
// see: https://pkg.go.dev/go.opentelemetry.io/otel/semconv/v1.7.0#pkg-constants
func NewTracer(serviceName string, opts ...func(*TracerOption)) (*sdktrace.TracerProvider, error) {

	// By default, always sample
	sampleRate, err := strconv.ParseFloat(config.LookupEnv("OTL_TRACE_SAMPLE_RATE", "1.0"), 64)
	if err != nil {
		return nil, errors.Wrap(err, "getting sample rate from OTL_TRACE_SAMPLE_RATE")
	}
	otlEndpoint := config.LookupEnv("OTL_ENDPOINT", "127.0.0.1:4317")

	// Default configuration
	option := &TracerOption{
		OtlEndpoint: otlEndpoint,
		SampleRate:  sampleRate,
	}
	for _, o := range opts {
		o(option)
	}

	ctx := context.Background()
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(option.OtlEndpoint),
	)
	if err != nil {
		return nil, err
	}

	bsp := sdktrace.NewBatchSpanProcessor(exporter)

	resource, err := newResource(serviceName)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(option.SampleRate))),
		sdktrace.WithSpanProcessor(bsp),
		sdktrace.WithResource(resource),
	)
	otel.SetTracerProvider(tp)

	propagator := b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader | b3.B3SingleHeader))
	otel.SetTextMapPropagator(propagator)

	return tp, nil
}

// TracerOption for the Tracer.
type TracerOption struct {
	OtlEndpoint string
	SampleRate  float64
}

// WithSampleRate set the sample rate of tracing.
// For example, set sample_rate to 1 if you wanna sampling 100% of trace data.
// Set 0.5 if you wanna sampling 50% of trace data, and so forth.
func WithSampleRate(rate float64) func(*TracerOption) {
	return func(o *TracerOption) {
		o.SampleRate = rate
	}
}

func WithOtelEndpoint(endpoint string) func(option *TracerOption) {
	return func(o *TracerOption) {
		o.OtlEndpoint = endpoint
	}
}
