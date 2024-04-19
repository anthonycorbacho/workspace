package telemetry

import (
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
)

// NewMeter configures the OpenTelemetry Resource and metrics exporter.
//
// A list of attributes can be passed via env variable OTEL_RESOURCE_ATTRIBUTES;
//
// eg:
//
//	OTEL_RESOURCE_ATTRIBUTES=service.version=0.0.1,service.namespace=default
//
// see: https://pkg.go.dev/go.opentelemetry.io/otel/semconv/v1.7.0#pkg-constants
func NewMeter(name string, opts ...func(option *MeterOption)) (*metric.MeterProvider, error) {

	// The exporter embeds a default OpenTelemetry Reader and
	// implements prometheus.Collector, allowing it to be used as
	// both a Reader and Collector.
	exporter, err := prometheus.New()
	if err != nil {
		return nil, err
	}

	resource, err := newResource(name)
	if err != nil {
		return nil, err
	}

	provider := metric.NewMeterProvider(
		metric.WithResource(resource),
		metric.WithReader(exporter),
	)

	otel.SetMeterProvider(provider)

	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		if err := http.ListenAndServe(":9090", mux); err != nil {
			log.Fatal(err)
		}
	}()
	return provider, nil
}

// MeterOption for the Meter.
type MeterOption struct {
}
