package main

import (
	"context"

	pb "github.com/anthonycorbacho/workspace/api/sample/sampleapp/v1"
	"github.com/anthonycorbacho/workspace/kit"
	"github.com/anthonycorbacho/workspace/kit/config"
	"github.com/anthonycorbacho/workspace/kit/id"
	"github.com/anthonycorbacho/workspace/kit/log"
	"github.com/anthonycorbacho/workspace/sample/sampleapp"
	"github.com/anthonycorbacho/workspace/sample/sampleapp/inmem"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
)

// Main function.
// Everything start from here.
func main() {
	podName := config.LookupEnv("POD_NAME", id.NewGenerator("sample-sampleapp").Generate())
	ctx := context.Background()
	// Initiate a logger with pre-configuration for production and telemetry.
	l, err := log.New()
	if err != nil {
		// in case we cannot create the logger, the app should immediately stop.
		panic(err)
	}
	// Replace the global logger with the Service scoped log.
	log.ReplaceGlobal(l)

	// Initialize service
	// Mostly business logic initialization will be there
	userService := sampleapp.New(inmem.New())
	srv, err := newGrpcUser(userService)
	if err != nil {
		l.Fatal(ctx, err.Error())
	}

	// Initialise the foundation and start the service
	foundation, err := kit.NewFoundation("sampleapp", kit.WithLogger(l))
	if err != nil {
		l.Fatal(ctx, err.Error())
	}

	// Register the GRPC Server.
	foundation.RegisterService(func(s *grpc.Server) {
		pb.RegisterSampleAppServer(s, srv)
	})

	// Register the Service Handler.
	foundation.RegisterServiceHandler(func(gw *runtime.ServeMux, conn *grpc.ClientConn) {
		if err := pb.RegisterSampleAppHandler(ctx, gw, conn); err != nil {
			l.Fatal(ctx, "fail registering gateway handler", log.Error(err))
		}
	})

	// Example: Register the HTTP handlers
	// 	userHandler := &HttpUser{
	//		service: userService,
	//		log:     l,
	//	}
	//foundation.RegisterHTTPHandler("/users/{id}", userHandler.User, "GET")
	//foundation.RegisterHTTPHandler("/users", userHandler.Create, "POST")
	//foundation.RegisterHTTPHandler("/users/{id}", userHandler.Delete, "DELETE")

	l.Info(ctx, "Starting service", log.String("pod.name", podName))

	// Start the service
	//
	// This service will be automatically configured to:
	// 1. Provide Observability information such as tracing, loging and metric
	// 2. Provide default /readyz and /healthz endpoint for rediness and liveness probe and profiling via /debug/pprof
	// 3. Setup for production setup
	// 4. Graceful shutdown
	if err := foundation.Serve(); err != nil {
		l.Error(ctx, "fail serving", log.Error(err))
	}
}
