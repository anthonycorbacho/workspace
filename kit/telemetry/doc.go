// Package telemetry creates the OpenTelemetry providers of a service, set as the global providers.
//
//	// Traces exported to the OTLP collector (OTEL_EXPORTER_OTLP_ENDPOINT).
//	tracer, err := telemetry.NewTracer("myservice")
//
//	// Metrics registered in the default prometheus registry.
//	meter, err := telemetry.NewMeter("myservice")
//
// The caller shuts down the providers before exiting to flush the pending data. The Foundation (see kit) creates
// both providers when it serves.
package telemetry
