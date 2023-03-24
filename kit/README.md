# Kit Framework
Kit is a framework for building production grade scalable service applications that can run in Kubernetes.

The main goal of Kit is to provide a proven starting point that reduces the repetitive tasks required for a new project.
It is designed to follow (at least try to) idiomatic code and Go best practices. Collectively, 
the project lays out everything logically to minimize guess work and enable engineers to quickly maintain a mental model for the project.

## Starting point

Starting building a new kit service is as simple as

```go
// Main function.
// Everything start from here.
func main() {
	podName := config.LookupEnv("POD_NAME", id.NewGenerator("myapp").Generate())
	ctx := context.Background()
	
	// Initiate a logger with pre-configuration for production and telemetry.
	l, err := log.New()
	if err != nil {
		// in case we cannot create the logger, the app should immediately stop.
		panic(err)
	}
	// Replace the global logger with the Service scoped log.
	log.ReplaceGlobal(l)
	
	// Initialise the foundation and start the service
	foundation, err := kit.NewFoundation(podName, kit.WithLogger(l))
	if err != nil {
		l.Fatal(ctx, err.Error())
	}
	
	l.Info(ctx, "Starting service", log.String("pod.name", podName))

	// Start the service
	//
	// This service will be automatically configured to:
	// 1. Provide Observability information such as tracing, loging and metric
	// 2. Provide default /readyz and /healthz endpoint for readiness and liveness probe and profiling via /debug/pprof
	// 3. Setup for production setup
	// 4. Graceful shutdown
	if err := foundation.Serve(); err != nil {
		l.Error(ctx, "fail serving", log.Error(err))
	}
}
```