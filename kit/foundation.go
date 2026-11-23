package kit

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/anthonycorbacho/workspace/kit/config"
	"github.com/anthonycorbacho/workspace/kit/errors"
	grpckit "github.com/anthonycorbacho/workspace/kit/grpc"
	"github.com/anthonycorbacho/workspace/kit/log"
	"github.com/anthonycorbacho/workspace/kit/telemetry"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/klauspost/compress/gzhttp"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"
	httpmetrics "github.com/slok/go-http-metrics/metrics"
	metrics "github.com/slok/go-http-metrics/metrics/prometheus"
	"github.com/slok/go-http-metrics/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// httpMetricsRecorder returns the HTTP metrics recorder, it is created once since its
// collectors are registered in the default prometheus registry.
var httpMetricsRecorder = sync.OnceValue(func() httpmetrics.Recorder {
	return metrics.NewRecorder(metrics.Config{})
})

// defaultHealthHandler provides a default health function.
var _defaultHealthHandler = func(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "text/plain")
	writer.WriteHeader(http.StatusOK)
	fmt.Fprintln(writer, "ok") //nolint
}

const (
	defaultHTTPReadHeaderTimeout = 5 * time.Second
	defaultHTTPIdleTimeout       = 60 * time.Second
	defaultHTTPMaxHeaderBytes    = 1 << 20
	maxQueryParamsMetadataBytes  = 4096
)

// Foundation provides a convenient way to build new services.
//
// Foundation aims to provide a set of common boilerplate code for creating a production ready GRPC server and
// HTTP mux router (with grpc-gateway capabilities) and custom HTTP endpoints.
type Foundation struct {
	// foundation name
	name string
	// foundation options
	opts *options
	// foundation logger
	logger *log.Logger
	// gRPC gateway
	gw       *runtime.ServeMux
	gwClient *grpc.ClientConn
	gwErr    error
	gwOnce   sync.Once
	// gRPC server
	grpcServer *grpc.Server
	grpcOnce   sync.Once
	// HTTP server
	httpServer  *http.Server
	httpRouter  *http.ServeMux
	httpMetrics middleware.Middleware
	httpOnce    sync.Once
	// Healths checks
	livenessProbe  http.HandlerFunc
	readinessProbe http.HandlerFunc
	draining       atomic.Bool
	// Shutdown hooks
	hooksMu       sync.Mutex
	shutdownHooks []func(context.Context) error
}

// NewFoundation creates a new foundation service.
// A list of configurable option can be passed as option and as env Variable
// eg:
//
//	// Setting up grpc server via option
//	kit.NewFoundation("myservice", kit.WithGRPCAddr("localhost:8089"))
//
//	// Setting up grpc server via Env
//	FOUNDATION_GRPC_ADDRESS=localhost:8089
//	kit.NewFoundation("myservice")
//
// Order of priority for option is as follows:
//
//	1- Default configuration
//	2- Env variable
//	3- Options
func NewFoundation(name string, optFns ...Option) (*Foundation, error) {
	if len(name) == 0 {
		return nil, errors.New("foundation name is required")
	}

	var shutdownDelay time.Duration
	if v := config.LookupEnv("FOUNDATION_SHUTDOWN_DELAY", ""); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, errors.Wrap(err, "parsing FOUNDATION_SHUTDOWN_DELAY")
		}
		shutdownDelay = d
	}

	// Setup default configuration
	opts := &options{
		httpAddr:              config.LookupEnv("FOUNDATION_HTTP_ADDRESS", "0.0.0.0:8080"),
		grpcAddr:              config.LookupEnv("FOUNDATION_GRPC_ADDRESS", "0.0.0.0:8081"),
		gatewayDialOpts:       []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
		metricsAddr:           config.LookupEnv("FOUNDATION_METRICS_ADDRESS", ":9090"),
		probesAddr:            config.LookupEnv("FOUNDATION_PROBES_ADDRESS", ":9091"),
		httpWriteTimeout:      15 * time.Second,
		httpReadTimeout:       15 * time.Second,
		httpReadHeaderTimeout: defaultHTTPReadHeaderTimeout,
		httpIdleTimeout:       defaultHTTPIdleTimeout,
		httpMaxHeaderBytes:    defaultHTTPMaxHeaderBytes,
		shutdownTimeout:       15 * time.Second,
		shutdownDelay:         shutdownDelay,
		logger:                log.NewNop(),
	}
	for _, o := range optFns {
		o(opts)
	}
	if err := validateOptions(opts); err != nil {
		return nil, err
	}

	// Create the Foundation service
	return &Foundation{
		name:           name,
		opts:           opts,
		logger:         opts.logger,
		readinessProbe: _defaultHealthHandler,
		livenessProbe:  _defaultHealthHandler,
	}, nil
}

func validateOptions(opts *options) error {
	if opts.shutdownTimeout <= 0 {
		return errors.New("shutdown timeout must be positive")
	}
	if opts.shutdownDelay < 0 {
		return errors.New("shutdown delay must not be negative")
	}
	if opts.httpReadTimeout < 0 {
		return errors.New("http read timeout must not be negative")
	}
	if opts.httpWriteTimeout < 0 {
		return errors.New("http write timeout must not be negative")
	}
	if opts.httpReadHeaderTimeout < 0 {
		return errors.New("http read header timeout must not be negative")
	}
	if opts.httpIdleTimeout < 0 {
		return errors.New("http idle timeout must not be negative")
	}
	if opts.httpMaxHeaderBytes < 0 {
		return errors.New("http max header bytes must not be negative")
	}
	return nil
}

// RegisterServiceFunc represents a function for registering a grpc service handler.
type RegisterServiceFunc func(s *grpc.Server)

// RegisterService registers a grpc service handler.
func (f *Foundation) RegisterService(fn RegisterServiceFunc) {
	// Create GRPC server only once
	f.grpcOnce.Do(func() {
		f.grpcServer = grpckit.NewServer(f.logger, f.opts.grpcServerOpts...)
	})
	fn(f.grpcServer)
}

// initHTTPServerOnce will initialize the HTTP server once.
func (f *Foundation) initHTTPServerOnce() {
	f.httpOnce.Do(func() {
		opts := f.opts
		name := f.name

		// create http router
		f.httpRouter = http.NewServeMux()

		// Provide Prometheus metric
		// The metrics measured are based on RED and/or Four golden signals,
		// follow standards and try to be measured in an efficient way.
		// The metric middleware is applied on each registered route in order to record
		// the route pattern instead of the url path.
		f.httpMetrics = middleware.New(middleware.Config{
			Service:  name,
			Recorder: httpMetricsRecorder(),
		})

		h := recoverHTTP(f.logger, f.httpRouter)

		// If cors is enabled, we should set it depending on the options
		if opts.cors != nil {
			h = cors.New(*opts.cors).Handler(h)
		}

		// Provide tracing for OTEL, the spans are named after the route pattern (see telemetry.Middleware).
		h = telemetry.TraceHandler(h, name)

		// Compress the responses
		h = gzhttp.GzipHandler(h)

		// create http server with options
		f.httpServer = &http.Server{
			Addr:              opts.httpAddr,
			Handler:           h,
			WriteTimeout:      opts.httpWriteTimeout,
			ReadTimeout:       opts.httpReadTimeout,
			ReadHeaderTimeout: opts.httpReadHeaderTimeout,
			IdleTimeout:       opts.httpIdleTimeout,
			MaxHeaderBytes:    opts.httpMaxHeaderBytes,
		}
	})

}

// RegisterServiceHandlerFunc represents a function for registering a grpc gateway service handler.
type RegisterServiceHandlerFunc func(gw *runtime.ServeMux, conn *grpc.ClientConn)

// RegisterServiceHandler registers a grpc-gateway service handler.
// The gateway's gRPC client is configured with WithGatewayDialOptions.
// Any provided muxOpts are applied after Foundation's defaults so callers can override them.
func (f *Foundation) RegisterServiceHandler(fn RegisterServiceHandlerFunc, muxOpts ...runtime.ServeMuxOption) {
	// Make sure we have an HTTP server setup
	f.initHTTPServerOnce()
	// Only create one time the gateway and grpc client
	f.gwOnce.Do(func() {
		f.logger.Info(context.Background(), "initializing grpc-gateway")
		conn, err := grpckit.NewClient(f.opts.grpcAddr, f.opts.gatewayDialOpts...)
		if err != nil {
			// reported by Serve, registering handlers with a nil connection would fail on every request.
			f.gwErr = errors.Wrap(err, "creating grpc client for grpc-gateway")
			return
		}

		f.gwClient = conn

		// Apply Foundation's defaults before caller options so callers can override
		// routing/error handlers, header matchers, marshalers, or metadata annotators.
		defaultOpts := []runtime.ServeMuxOption{
			runtime.WithRoutingErrorHandler(gatewayRoutingErrorHandler(f.logger)),
			runtime.WithErrorHandler(gatewayErrorHandler(f.logger)),
			runtime.WithMiddlewares(
				telemetry.GatewayMiddleware(f.httpMetrics),
				recoverGatewayMiddleware(f.logger),
			),
			runtime.WithIncomingHeaderMatcher(incomingHeaderMatcher(f.opts.gatewayAllowedHeaders)),
			runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.HTTPBodyMarshaler{
				Marshaler: &runtime.JSONPb{
					UseProtoNames:   true,
					EmitUnpopulated: true,
				},
			}),
			runtime.WithMetadata(gatewayMetadata(f.logger)),
		}
		f.gw = runtime.NewServeMux(append(defaultOpts, muxOpts...)...)
	})

	if f.gwErr != nil {
		return
	}
	fn(f.gw, f.gwClient)
}

func incomingHeaderMatcher(allowed map[string]struct{}) runtime.HeaderMatcherFunc {
	if allowed != nil {
		return func(s string) (string, bool) {
			if _, ok := allowed[http.CanonicalHeaderKey(s)]; ok {
				return s, true
			}
			return runtime.DefaultHeaderMatcher(s)
		}
	}
	return func(s string) (string, bool) {
		// Allowing passing custom headers by default when no allowlist is configured.
		if strings.HasPrefix(s, "X-") {
			return s, true
		}
		return runtime.DefaultHeaderMatcher(s)
	}
}

func gatewayMetadata(l *log.Logger) func(context.Context, *http.Request) metadata.MD {
	return func(ctx context.Context, req *http.Request) metadata.MD {
		md := make(map[string]string, 3)
		if pattern, ok := runtime.HTTPPathPattern(ctx); ok {
			md["pattern"] = pattern
		}
		md["method"] = req.Method

		if req.URL.RawQuery == "" {
			md["query-params"] = "{}"
		} else if queryParams, err := json.Marshal(req.URL.Query()); err == nil {
			if len(queryParams) <= maxQueryParamsMetadataBytes {
				md["query-params"] = string(queryParams)
			} else {
				l.Warn(ctx, "skipping oversized query-params gateway metadata", log.Int("bytes", len(queryParams)))
			}
		}

		return metadata.New(md)
	}
}

// RegisterHTTPHandler registers a custom HTTP handler.
//
// The path follows the http.ServeMux pattern syntax, path parameters can be read with r.PathValue
// and the handler is registered for each of the given methods (or for all methods if none given).
//
// eg:
//
//	foundation.RegisterHTTPHandler("/users/{id}", func(w http.ResponseWriter, r *http.Request) {
//		id := r.PathValue("id")
//	}, http.MethodGet)
func (f *Foundation) RegisterHTTPHandler(path string, fn http.HandlerFunc, methods ...string) {
	// make sure the HTTP server has been initialized
	f.initHTTPServerOnce()
	h := telemetry.Middleware(f.httpMetrics)(recoverHTTP(f.logger, fn))
	if len(methods) == 0 {
		f.httpRouter.Handle(path, h)
		return
	}
	for _, m := range methods {
		f.httpRouter.Handle(m+" "+path, h)
	}
}

// RegisterShutdown registers a cleanup function invoked during shutdown after the HTTP and gRPC
// servers have drained and before the probes and metrics servers close.
// Hooks run in reverse registration order (LIFO) within the shared WithShutdownTimeout budget.
// A nil function is ignored.
func (f *Foundation) RegisterShutdown(fn func(context.Context) error) {
	if fn == nil {
		return
	}
	f.hooksMu.Lock()
	f.shutdownHooks = append(f.shutdownHooks, fn)
	f.hooksMu.Unlock()
}

// RegisterLiveness register a liveness function for /healthz
//
// Many applications running for long periods of time eventually transition to broken states,
// and cannot recover except by being restarted.
// Kubernetes provides liveness probes to detect and remedy such situations.
func (f *Foundation) RegisterLiveness(fn func() (string, error)) {
	if fn == nil {
		return
	}
	f.RegisterLivenessCheck(func(context.Context) (string, error) { return fn() })
}

// RegisterLivenessCheck registers a context-aware liveness function for /healthz.
// The context is canceled if the probe client disconnects or times out.
func (f *Foundation) RegisterLivenessCheck(fn func(context.Context) (string, error)) {
	if fn == nil {
		return
	}
	f.livenessProbe = handlerClosure(fn)
}

// RegisterReadiness register a readiness function for /readyz
//
// Sometimes, applications are temporarily unable to serve traffic.
// For example, an application might need to load a large amount of data or
// a large number of configuration files during startup.
// In such instances, we don’t want to kill the application, but we don’t want to send it requests either.
func (f *Foundation) RegisterReadiness(fn func() (string, error)) {
	if fn == nil {
		return
	}
	f.RegisterReadinessCheck(func(context.Context) (string, error) { return fn() })
}

// RegisterReadinessCheck registers a context-aware readiness function for /readyz.
// The context is canceled if the probe client disconnects or times out.
func (f *Foundation) RegisterReadinessCheck(fn func(context.Context) (string, error)) {
	if fn == nil {
		return
	}
	f.readinessProbe = handlerClosure(fn)
}

func handlerClosure(fn func(context.Context) (string, error)) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		status := http.StatusOK
		msg, err := fn(request.Context())
		if err != nil {
			status = http.StatusInternalServerError
			msg = err.Error()
		}
		writer.Header().Set("Content-Type", "text/plain")
		writer.WriteHeader(status)
		fmt.Fprintln(writer, msg) //nolint
	}
}

// Serve configure and start serving request for the foundation service.
//
// It blocks until the process receives an interrupt or terminate signal, or until one of the servers fails.
// Shutdown marks the service unready and drains HTTP before gRPC, within the shared WithShutdownTimeout budget.
func (f *Foundation) Serve() error {
	return f.ServeContext(context.Background())
}

// ServeContext configures and starts serving requests for the foundation service.
//
// It blocks until ctx is canceled, the process receives an interrupt or terminate signal, or one of the servers fails.
// Returning due to ctx cancellation or OS signal triggers graceful shutdown and returns nil.
func (f *Foundation) ServeContext(ctx context.Context) error {
	// GOMAXPROCS is container aware since go 1.25 (cgroup CPU limit), no need to set it.
	if ctx == nil {
		ctx = context.Background()
	}

	if f.gwErr != nil {
		return f.gwErr
	}

	// Setup telemetry
	tracer, err := telemetry.NewTracer(f.name)
	if err != nil {
		return errors.Wrap(err, "creating new tracer")
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tracer.Shutdown(shutdownCtx); err != nil {
			f.logger.Warn(shutdownCtx, "shutting down tracer", log.Error(err))
		}
	}()

	meter, err := telemetry.NewMeter(f.name)
	if err != nil {
		return errors.Wrap(err, "creating new meter")
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := meter.Shutdown(shutdownCtx); err != nil {
			f.logger.Warn(shutdownCtx, "shutting down meter", log.Error(err))
		}
	}()

	// health probes, profiling and metrics are served on their own servers,
	// so they are not exposed with the public HTTP server.
	// WriteTimeout is intentionally left unset so pprof CPU profiles (default 30s) can stream to completion.
	internalServers := []*http.Server{
		{
			Addr:              f.opts.probesAddr,
			Handler:           internalRouter(f.handleReadiness, f.livenessProbe),
			ReadTimeout:       15 * time.Second,
			ReadHeaderTimeout: defaultHTTPReadHeaderTimeout,
			IdleTimeout:       defaultHTTPIdleTimeout,
			MaxHeaderBytes:    defaultHTTPMaxHeaderBytes,
		},
		{
			Addr:              f.opts.metricsAddr,
			Handler:           metricsRouter(),
			ReadTimeout:       15 * time.Second,
			ReadHeaderTimeout: defaultHTTPReadHeaderTimeout,
			IdleTimeout:       defaultHTTPIdleTimeout,
			MaxHeaderBytes:    defaultHTTPMaxHeaderBytes,
		},
	}

	// Listen for an interrupt or terminate signal from the OS, or caller context cancellation.
	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Make a channel to listen for errors coming from the listeners. It is buffered
	// for every server so the goroutines can exit if we don't collect their error.
	serverError := make(chan error, len(internalServers)+2)

	for _, srv := range internalServers {
		go func() {
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				serverError <- errors.Wrapf(err, "internal server %s", srv.Addr)
			}
		}()
	}

	if err := f.startGRPCServer(serverError); err != nil {
		f.drain(false, internalServers...)
		return err
	}
	f.startHTTPServer(serverError)

	f.logger.Info(context.Background(), "service started", log.String("service-name", f.name))

	var serveErr error
	select {
	case serveErr = <-serverError:
		f.drain(false, internalServers...)
	case <-sigCtx.Done():
		f.shutdown(internalServers...)
	}

	return serveErr
}

// startGRPCServer starts the gRPC server if services were registered,
// the serving errors are sent to serverError.
func (f *Foundation) startGRPCServer(serverError chan<- error) error {
	if f.grpcServer == nil {
		return nil
	}

	// enable grpc metrics
	// This operation needs to be done after user register the proto to the server.
	grpckit.InitializeMetrics(f.grpcServer)

	// Create listener for the grpc server
	listen, err := net.Listen("tcp", f.opts.grpcAddr)
	if err != nil {
		return errors.Wrap(err, "init net listener")
	}
	go func() {
		// Serve closes the listener when it returns.
		if err := f.grpcServer.Serve(listen); err != nil {
			serverError <- errors.Wrap(err, "grpc server")
		}
	}()
	return nil
}

// startHTTPServer starts the HTTP server if handlers were registered,
// the serving errors are sent to serverError.
func (f *Foundation) startHTTPServer(serverError chan<- error) {
	if f.httpServer == nil {
		return
	}
	if f.gw != nil {
		// the gateway handles all the requests not matching a registered HTTP handler.
		f.httpRouter.Handle("/", f.gw)
	}
	go func() {
		if err := f.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverError <- errors.Wrap(err, "http server")
		}
	}()
}

// metricsRouter returns the router exposing the prometheus metrics.
func metricsRouter() *http.ServeMux {
	r := http.NewServeMux()
	r.Handle("GET /metrics", promhttp.Handler())
	return r
}

// internalRouter returns the router for health checks and profiling.
func internalRouter(readiness http.HandlerFunc, liveliness http.HandlerFunc) *http.ServeMux {
	r := http.NewServeMux()

	// Init default health checks.
	r.HandleFunc("GET /healthz", liveliness)
	r.HandleFunc("GET /readyz", readiness)

	// pprof
	r.HandleFunc("/debug/pprof/", pprof.Index)
	r.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	r.HandleFunc("/debug/pprof/profile", pprof.Profile)
	r.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	r.HandleFunc("/debug/pprof/trace", pprof.Trace)

	r.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	r.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	r.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
	r.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	r.Handle("/debug/pprof/block", pprof.Handler("block"))
	r.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))

	return r
}
