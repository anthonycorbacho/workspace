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

### Add gRPC service and gRPC gateway
Foundation comes with integration with GRPC (GRPC gateway) out of the box.
You can define your protobuf API in [api folder](../api).
Once you generated the code via `make proto-gen`, you can start creating your application as shown below:

```go
func main() {
	// Initialise the foundation
	foundation, err := kit.NewFoundation("myservice")
	if err != nil { 
		// handle error 
	}
  
	// Register the GRPC Server
	foundation.RegisterService(func(s *grpc.Server) {
    	// since in a closure, you only need to pass a RegisterServiceFunc
    	// with the proto Register App and the struct that implement the interfaces
    	// the underline implementation and setup of the GRPC server
    	// will be managed by the foundation.
		pb.RegisterMyAppServer(s, &myStruct{})
	})
  
	// Register the Service Handler in case you want to expose your GRPC service via HTTP
	// it use underneath grpc-gateway.
	// like RegisterService, RegisterServiceHandler takes a RegisterServiceHandlerFunc
	// with the proto Register App Handler and the underline implementation and setup
	// will be managed by the foundation.
	foundation.RegisterServiceHandler(func(gw *runtime.ServeMux, conn *grpc.ClientConn) {
		if err := pb.RegisterMyAppHandler(ctx, gw, conn); err != nil {
			// handle error
		}
	})
  
  	// Start the service
  	if err := foundation.Serve(); err != nil {
		// handle error
  	}
}
```
