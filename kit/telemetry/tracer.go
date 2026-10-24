package telemetry

import (
	"cmp"
	"context"
	"crypto/tls"
	"os"
	"strconv"

	"github.com/anthonycorbacho/workspace/kit/errors"
	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc/credentials"
)

// _defaultOTLPEndpoint is the collector endpoint used when none is configured, without TLS.
const _defaultOTLPEndpoint = "127.0.0.1:4317"

// tracerOptions are the configurable options of NewTracer.
type tracerOptions struct {
	endpoint   string
	sampleRate float64
	tlsConfig  *tls.Config
}

// TracerOption configures the TracerProvider created by NewTracer.
type TracerOption func(*tracerOptions)

// WithSampleRate sets the ratio of the sampled root traces, from 0 (none) to 1 (all), it overrides
// OTEL_TRACES_SAMPLER_ARG. The child spans follow the sampling decision of their parent.
func WithSampleRate(rate float64) TracerOption {
	return func(o *tracerOptions) {
		o.sampleRate = rate
	}
}

// WithOTLPEndpoint sets the host:port of the OTLP gRPC collector, it overrides OTEL_EXPORTER_OTLP_ENDPOINT.
// The connection is not encrypted unless WithTLSConfig is given.
func WithOTLPEndpoint(endpoint string) TracerOption {
	return func(o *tracerOptions) {
		o.endpoint = endpoint
	}
}

// WithTLSConfig encrypts the connection to the collector with cfg.
func WithTLSConfig(cfg *tls.Config) TracerOption {
	return func(o *tracerOptions) {
		o.tlsConfig = cfg
	}
}

// NewTracer returns a TracerProvider exporting the spans to an OTLP gRPC collector, set as the global
// provider with the W3C trace context, W3C baggage and B3 propagators.
//
// The exporter is configured with the standard OpenTelemetry environment variables:
//
//   - OTEL_EXPORTER_OTLP_ENDPOINT (or OTEL_EXPORTER_OTLP_TRACES_ENDPOINT): the collector URL, eg:
//     http://alloy.monitoring.svc.cluster.local:4317 (an http scheme disables TLS), the other
//     OTEL_EXPORTER_OTLP_* variables (headers, certificates, timeout...) are supported too.
//   - OTEL_TRACES_SAMPLER_ARG: the ratio of the sampled root traces, 1 (all) by default.
//   - OTEL_RESOURCE_ATTRIBUTES: the attributes of the service, eg: service.version=0.0.1,service.namespace=default
//
// The collector is 127.0.0.1:4317 without TLS when no endpoint is configured. OTL_ENDPOINT and
// OTL_TRACE_SAMPLE_RATE are still read when the standard variables are not set.
func NewTracer(serviceName string, opts ...TracerOption) (*sdktrace.TracerProvider, error) {
	sampleRate, err := envSampleRate()
	if err != nil {
		return nil, err
	}
	o := &tracerOptions{sampleRate: sampleRate}
	for _, opt := range opts {
		opt(o)
	}

	// the resource is created first so a failure doesn't leave an exporter opened.
	resource, err := newResource(serviceName)
	if err != nil {
		return nil, err
	}

	exporter, err := otlptracegrpc.New(context.Background(), exporterOptions(o)...)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(o.sampleRate))),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
		b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader|b3.B3SingleHeader)),
	))

	return tp, nil
}

// envSampleRate returns the sample rate from OTEL_TRACES_SAMPLER_ARG, or OTL_TRACE_SAMPLE_RATE, 1 by default.
func envSampleRate() (float64, error) {
	for _, key := range []string{"OTEL_TRACES_SAMPLER_ARG", "OTL_TRACE_SAMPLE_RATE"} {
		v := os.Getenv(key)
		if v == "" {
			continue
		}
		rate, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, errors.Wrapf(err, "parsing the sample rate %s", key)
		}
		return rate, nil
	}
	return 1, nil
}

// exporterOptions returns the options of the exporter, the standard environment variables are read by the
// exporter and the options override them.
func exporterOptions(o *tracerOptions) []otlptracegrpc.Option {
	var opts []otlptracegrpc.Option
	if endpoint := exporterEndpoint(o); endpoint != "" {
		opts = append(opts, otlptracegrpc.WithEndpoint(endpoint))
		if o.tlsConfig == nil {
			opts = append(opts, otlptracegrpc.WithInsecure())
		}
	}
	if o.tlsConfig != nil {
		opts = append(opts, otlptracegrpc.WithTLSCredentials(credentials.NewTLS(o.tlsConfig)))
	}
	return opts
}

// exporterEndpoint returns the endpoint of the collector, empty when it is read by the exporter from the
// standard environment variables.
func exporterEndpoint(o *tracerOptions) string {
	if o.endpoint != "" || hasEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") {
		return o.endpoint
	}
	return cmp.Or(os.Getenv("OTL_ENDPOINT"), _defaultOTLPEndpoint)
}

// hasEnv reports whether one of the environment variables is set and not empty.
func hasEnv(keys ...string) bool {
	for _, k := range keys {
		if os.Getenv(k) != "" {
			return true
		}
	}
	return false
}
