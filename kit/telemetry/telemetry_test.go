package telemetry

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/anthonycorbacho/workspace/kit/id"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

func TestNewResource(t *testing.T) {
	tests := []struct {
		name      string
		env       string
		wantAttrs map[attribute.Key]string
		wantErr   bool
	}{
		{
			name:      "service name",
			wantAttrs: map[attribute.Key]string{semconv.ServiceNameKey: "myservice"},
		},
		{
			name: "attributes from env",
			env:  "service.version=0.0.1,service.namespace=default",
			wantAttrs: map[attribute.Key]string{
				semconv.ServiceNameKey:      "myservice",
				semconv.ServiceVersionKey:   "0.0.1",
				semconv.ServiceNamespaceKey: "default",
			},
		},
		{name: "malformed env", env: "service.version", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OTEL_RESOURCE_ATTRIBUTES", tt.env)
			res, err := newResource("myservice")
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			set := res.Set()
			for k, want := range tt.wantAttrs {
				got, ok := set.Value(k)
				if assert.True(t, ok, "attribute %s", k) {
					assert.Equal(t, want, got.AsString())
				}
			}
			_, ok := set.Value(semconv.HostNameKey)
			assert.True(t, ok, "host attributes")
		})
	}
}

func TestTracerOptions(t *testing.T) {
	tests := []struct {
		name string
		opts []func(*TracerOption)
		want TracerOption
	}{
		{name: "no options", want: TracerOption{OtlEndpoint: "default:4317", SampleRate: 1}},
		{name: "sample rate", opts: []func(*TracerOption){WithSampleRate(0.5)}, want: TracerOption{OtlEndpoint: "default:4317", SampleRate: 0.5}},
		{name: "endpoint", opts: []func(*TracerOption){WithOtelEndpoint("collector:4317")}, want: TracerOption{OtlEndpoint: "collector:4317", SampleRate: 1}},
		{
			name: "last option wins",
			opts: []func(*TracerOption){WithSampleRate(0.5), WithSampleRate(0), WithOtelEndpoint("a:1"), WithOtelEndpoint("b:2")},
			want: TracerOption{OtlEndpoint: "b:2", SampleRate: 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := TracerOption{OtlEndpoint: "default:4317", SampleRate: 1}
			for _, opt := range tt.opts {
				opt(&o)
			}
			assert.Equal(t, tt.want, o)
		})
	}
}

func TestNewTracer(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		opts        []func(*TracerOption)
		wantSampled bool
		wantErr     bool
	}{
		{name: "default always samples", wantSampled: true},
		{name: "sample rate option", opts: []func(*TracerOption){WithSampleRate(0)}},
		{name: "sample rate from env", env: map[string]string{"OTL_TRACE_SAMPLE_RATE": "0"}},
		{name: "option overrides env", env: map[string]string{"OTL_TRACE_SAMPLE_RATE": "0"}, opts: []func(*TracerOption){WithSampleRate(1)}, wantSampled: true},
		{name: "endpoint option", opts: []func(*TracerOption){WithOtelEndpoint("127.0.0.1:14317")}, wantSampled: true},
		{name: "endpoint from env", env: map[string]string{"OTL_ENDPOINT": "127.0.0.1:14317"}, wantSampled: true},
		{name: "invalid sample rate env", env: map[string]string{"OTL_TRACE_SAMPLE_RATE": "always"}, wantErr: true},
		{name: "malformed resource env", env: map[string]string{"OTEL_RESOURCE_ATTRIBUTES": "service.version"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, k := range []string{"OTL_TRACE_SAMPLE_RATE", "OTL_ENDPOINT", "OTEL_RESOURCE_ATTRIBUTES"} {
				t.Setenv(k, "")
				require.NoError(t, os.Unsetenv(k))
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			tp, err := NewTracer("myservice", tt.opts...)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, tp)
				return
			}
			require.NoError(t, err)
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				_ = tp.Shutdown(ctx)
			})

			assert.Equal(t, tp, otel.GetTracerProvider(), "global tracer provider is set")
			fields := otel.GetTextMapPropagator().Fields()
			assert.Contains(t, fields, "b3")
			assert.Contains(t, fields, "x-b3-traceid")

			// spans are not ended so nothing is exported to the (unavailable) collector.
			_, span := tp.Tracer("test").Start(context.Background(), "span")
			assert.True(t, span.SpanContext().IsValid())
			assert.Equal(t, tt.wantSampled, span.SpanContext().IsSampled())
		})
	}
}

func TestNewMeter(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		wantErr bool
	}{
		{name: "meter"},
		{name: "meter with resource env", env: "service.version=0.0.1"},
		{name: "malformed resource env", env: "service.version", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OTEL_RESOURCE_ATTRIBUTES", tt.env)
			mp, err := NewMeter("myservice")
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, mp)
				return
			}
			require.NoError(t, err)
			// the exporter stays registered in the default prometheus registry,
			// shutting down the provider makes it stop exposing metrics.
			t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
			assert.Equal(t, mp, otel.GetMeterProvider(), "global meter provider is set")

			name := "kit_test_meter_" + id.New()
			counter, err := mp.Meter("test").Int64Counter(name)
			require.NoError(t, err)
			counter.Add(context.Background(), 3)

			families, err := prometheus.DefaultGatherer.Gather()
			require.NoError(t, err)
			var found bool
			for _, f := range families {
				if !strings.HasPrefix(f.GetName(), name) {
					continue
				}
				found = true
				require.Len(t, f.GetMetric(), 1)
				assert.Equal(t, float64(3), f.GetMetric()[0].GetCounter().GetValue())
			}
			assert.True(t, found, "metric must be exposed in the default prometheus registry")
		})
	}
}
