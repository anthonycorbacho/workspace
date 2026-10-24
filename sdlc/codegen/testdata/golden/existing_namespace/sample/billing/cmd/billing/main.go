package main

import (
	"context"
	"fmt"
	"os"

	"example.com/acme/workspace/kit"
	"example.com/acme/workspace/kit/log"
)

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

// run starts the service and blocks until it is stopped, the errors are logged before being returned.
func run() error {
	ctx := context.Background()

	// Initiate a logger with pre-configuration for production and telemetry.
	l, err := log.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating logger: %v\n", err)
		return err
	}
	defer l.Close()
	// Replace the global logger with the service scoped log.
	log.ReplaceGlobal(l)

	foundation, err := kit.NewFoundation("billing", kit.WithLogger(l))
	if err != nil {
		l.Error(ctx, "creating foundation", log.Error(err))
		return err
	}

	// Register the gRPC services and their gateway handlers, eg:
	//
	//	foundation.RegisterService(func(s *grpc.Server) {
	//		pb.RegisterBillingServiceServer(s, srv)
	//	})
	//	foundation.RegisterServiceHandler(func(gw *runtime.ServeMux, conn *grpc.ClientConn) {
	//		_ = pb.RegisterBillingServiceHandler(ctx, gw, conn)
	//	})

	l.Info(ctx, "starting service")
	// Serve blocks until the process receives an interrupt or terminate signal.
	if err := foundation.Serve(); err != nil {
		l.Error(ctx, "serving", log.Error(err))
		return err
	}
	return nil
}
