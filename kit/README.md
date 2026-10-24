# Kit Framework
Kit is a framework for building production grade scalable service applications that can run in Kubernetes.

The main goal of Kit is to provide a proven starting point that reduces the repetitive tasks required for a new project.
It is designed to follow idiomatic code and Go best practices. Collectively, the project lays out everything logically
to minimize guess work and enable engineers to quickly maintain a mental model for the project.

## Starting point

A Kit service is a `kit.Foundation`, see [sample/sampleapp](../sample/sampleapp/cmd/sampleapp/main.go) for a complete
service and `make codegen` to scaffold a new one.

```go
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
	log.ReplaceGlobal(l)

	foundation, err := kit.NewFoundation("myservice", kit.WithLogger(l))
	if err != nil {
		l.Error(ctx, "creating foundation", log.Error(err))
		return err
	}

	// Serve blocks until the process receives an interrupt or terminate signal, the service is:
	// 1. instrumented: traces (OpenTelemetry), metrics (Prometheus) and logs with the trace ids
	// 2. probed: /healthz and /readyz endpoints, and profiled via /debug/pprof
	// 3. gracefully shut down
	if err := foundation.Serve(); err != nil {
		l.Error(ctx, "serving", log.Error(err))
		return err
	}
	return nil
}
```

### Add a gRPC service and the gRPC gateway
The Foundation serves gRPC services and exposes them over HTTP with grpc-gateway.
Define the protobuf API in the [api folder](../api), generate the code with `make proto-gen`, then register the
service and its gateway handler:

```go
foundation.RegisterService(func(s *grpc.Server) {
	pb.RegisterMyAppServiceServer(s, server)
})

// the handler is registered synchronously, the error is checked right after.
var gatewayErr error
foundation.RegisterServiceHandler(func(gw *runtime.ServeMux, conn *grpc.ClientConn) {
	gatewayErr = pb.RegisterMyAppServiceHandler(ctx, gw, conn)
})
if gatewayErr != nil {
	return gatewayErr
}
```

The requests are validated with their protovalidate rules, and the handlers return coded errors
(see [errors](errors) and [application-error.md](../sdlc/documentation/application-error.md)).

### Configuration
The Foundation is configured with options, or environment variables (the options take precedence):

| Option                                   | Environment variable         | Default        |
|------------------------------------------|------------------------------|----------------|
| `WithHTTPAddr`                           | `FOUNDATION_HTTP_ADDRESS`    | `0.0.0.0:8080` |
| `WithGRPCAddr`                           | `FOUNDATION_GRPC_ADDRESS`    | `0.0.0.0:8081` |
| `WithMetricsAddr`                        | `FOUNDATION_METRICS_ADDRESS` | `:9090`        |
| `WithProbesAddr`                         | `FOUNDATION_PROBES_ADDRESS`  | `:9091`        |
| `WithLogger`                             | `FOUNDATION_LOG_LEVEL` (log) | no-op logger   |
| `WithCORS`, `WithHTTPReadTimeout`, `WithHTTPWriteTimeout`, `WithGRPCServerOptions` | | |

The traces are exported to the OTLP collector set by `OTEL_EXPORTER_OTLP_ENDPOINT` (see [telemetry](telemetry)).

## Packages

| Package                       | Purpose                                                              |
|-------------------------------|----------------------------------------------------------------------|
| [blob](blob)                  | Object storage (MinIO / S3)                                          |
| [cache](cache)                | Key value cache (Redis)                                              |
| [config](config)              | Configuration from YAML files (configmaps) and environment variables |
| [errors](errors)              | Coded errors, generated from `errors/errors.defs.yaml` (`make errgen`) |
| [grpc](grpc)                  | gRPC server and client: errors, validation, metrics, traces, retries |
| [id](id)                      | Globally unique identifiers                                          |
| [namesgenerator](namesgenerator) | Random names                                                      |
| [log](log)                    | Structured logging with the trace ids                                |
| [pagination](pagination)      | Next page token pagination and list batching                        |
| [password](password)          | High-entropy random password generation                              |
| [pubsub](pubsub)              | Publish and subscribe (GCP Pub/Sub, NATS, Kafka, MQTT)               |
| [retry](retry)                | Retries with backoff                                                 |
| [sql](sql)                    | PostgreSQL connection, migrations and test databases                 |
| [telemetry](telemetry)        | OpenTelemetry tracer and meter providers, HTTP metrics               |
| [validator](validator)        | Struct validation                                                    |
| [workerpool](workerpool)      | Bounded pool of goroutines                                           |

## Tests
`make test` runs the unit tests, the tests needing a service (PostgreSQL, Redis, NATS, Pub/Sub emulator, Kafka, MQTT,
MinIO) are skipped. `make test-integration` starts the services with docker compose and runs all the tests.
