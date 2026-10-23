package telemetry

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
)

// NewMeter configures the OpenTelemetry Resource and metrics exporter.
// The metrics are registered in the default prometheus registry, it is the caller responsibility to expose them
// (eg: promhttp.Handler()) and to shut down the returned provider.
//
// A list of attributes can be passed via env variable OTEL_RESOURCE_ATTRIBUTES;
//
// eg:
//
//	OTEL_RESOURCE_ATTRIBUTES=service.version=0.0.1,service.namespace=default
//
// see: https://pkg.go.dev/go.opentelemetry.io/otel/semconv/v1.7.0#pkg-constants
func NewMeter(name string, opts ...func(option *MeterOption)) (*metric.MeterProvider, error) {

	// the resource is created first so a failure doesn't leave the exporter
	// registered in the default prometheus registry.
	resource, err := newResource(name)
	if err != nil {
		return nil, err
	}

	// The exporter embeds a default OpenTelemetry Reader and
	// implements prometheus.Collector, allowing it to be used as
	// both a Reader and Collector.
	exporter, err := prometheus.New()
	if err != nil {
		return nil, err
	}

	provider := metric.NewMeterProvider(
		metric.WithResource(resource),
		metric.WithReader(exporter),
	)

	otel.SetMeterProvider(provider)
	return provider, nil
}

// MeterOption for the Meter.
type MeterOption struct {
}
