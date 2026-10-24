// Package kit provides the Foundation, the boilerplate of a production ready service: a gRPC server, an HTTP
// server with the grpc-gateway handlers and custom routes, the probes and metrics servers, the tracer and meter
// providers, and the graceful shutdown.
//
//	f, err := kit.NewFoundation("myservice", kit.WithLogger(l))
//	if err != nil {
//		return err
//	}
//	f.RegisterService(func(s *grpc.Server) {
//		pb.RegisterMyServiceServer(s, server)
//	})
//	f.RegisterServiceHandler(func(gw *runtime.ServeMux, conn *grpc.ClientConn) {
//		gatewayErr = pb.RegisterMyServiceHandler(context.Background(), gw, conn)
//	})
//	return f.Serve()
//
// The servers listen on 0.0.0.0:8080 (HTTP), 0.0.0.0:8081 (gRPC), :9090 (metrics) and :9091 (probes) by default,
// see the With* options and the FOUNDATION_* environment variables.
//
// The sub packages are independent building blocks (errors, log, sql, pubsub, cache, blob...) usable without
// the Foundation.
package kit
