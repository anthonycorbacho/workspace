// Package telemetry provides functions for managing
// instrumented code and measure data about that code's
// performance and operation.
//
// You can setup instrumentation by creating providers
//
//		// Create Tracer.
//		tracer, _ := telemetry.NewTracer("myservicename")
//
//		// Create a Meter.
//		meter, _ := telemetry.NewMeter("myservicename")
//
package telemetry
