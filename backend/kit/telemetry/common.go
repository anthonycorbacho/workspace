package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.18.0"
)

// newResource return a new OTEL resource.
// It read from env variable OTEL_RESOURCE_ATTRIBUTES for custom setting.
func newResource(name string) (*resource.Resource, error) {
	return resource.New(
		context.Background(),
		resource.WithHost(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(name),
		),
		resource.WithFromEnv(),
	)
}
