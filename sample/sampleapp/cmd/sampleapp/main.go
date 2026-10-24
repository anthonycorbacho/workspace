package main

import (
	"context"
	"fmt"
	"os"

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

	userService := sampleapp.New(inmem.New())
	srv, err := newGrpcUser(userService)
	if err != nil {
		l.Error(ctx, "creating grpc user service", log.Error(err))
		return err
	}

	foundation, err := kit.NewFoundation("sampleapp", kit.WithLogger(l))
	if err != nil {
		l.Error(ctx, "creating foundation", log.Error(err))
		return err
	}

	foundation.RegisterService(func(s *grpc.Server) {
		pb.RegisterSampleAppServiceServer(s, srv)
	})
	// the handler is registered synchronously, the error is checked right after.
	var gatewayErr error
	foundation.RegisterServiceHandler(func(gw *runtime.ServeMux, conn *grpc.ClientConn) {
		gatewayErr = pb.RegisterSampleAppServiceHandler(ctx, gw, conn)
	})
	if gatewayErr != nil {
		l.Error(ctx, "registering gateway handler", log.Error(gatewayErr))
		return gatewayErr
	}

	podName := config.LookupEnv("POD_NAME", id.NewGenerator("sample-sampleapp").Generate())
	l.Info(ctx, "starting service", log.String("pod.name", podName))
	// Serve blocks until the process receives an interrupt or terminate signal.
	if err := foundation.Serve(); err != nil {
		l.Error(ctx, "serving", log.Error(err))
		return err
	}
	return nil
}
