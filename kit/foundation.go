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
	"syscall"
	"time"

	"github.com/anthonycorbacho/workspace/kit/config"
	"github.com/anthonycorbacho/workspace/kit/errors"
	grpckit "github.com/anthonycorbacho/workspace/kit/grpc"
	"github.com/anthonycorbacho/workspace/kit/log"
	"github.com/anthonycorbacho/workspace/kit/telemetry"
	handlers "github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	grpcprometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/rs/cors"
	metrics "github.com/slok/go-http-metrics/metrics/prometheus"
	"github.com/slok/go-http-metrics/middleware"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gorilla/mux/otelmux"
	"go.uber.org/automaxprocs/maxprocs"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
)

// defaultHealthHandler provides a default health function.
var _defaultHealthHandler = func(writer http.ResponseWriter, _ *http.Request) {
	writer.WriteHeader(http.StatusOK)
	writer.Header().Set("Content-Type", "text/plain")
	fmt.Fprintln(writer, "ok") //nolint
}

// Foundation provides a convenient way to build new services.
//
// Foundation aims to provide a set of common boilerplate code for creating a production ready GRPC server and
// HTTP mux router (with grpc-gateway capabilities) and custom HTTP endpoints.
type Foundation struct {
	// foundation name
	name string
	// foundation options
	opts *FoundationOptions
	// foundation logger
	logger *log.Logger
	// gRPC gateway
	gw       *runtime.ServeMux
	gwClient *grpc.ClientConn
	gwOnce   sync.Once
	// gRPC server
	grpcServer *grpc.Server
	grpcOnce   sync.Once
	// HTTP server
	httpServer *http.Server
	httpRouter *mux.Router
	httpOnce   sync.Once
	// Healths checks
	livenessProbe  http.HandlerFunc
	readinessProbe http.HandlerFunc
}

// NewFoundation creates a new foundation service.
// A list of configurable option can be passed as option and as env Variable
// eg:
//
//	// Setting up grpc server via option
//	kit.NewFoundation("myservice", kit.WithGrpcAddr("localhost:8089"))
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
func NewFoundation(name string, options ...Option) (*Foundation, error) {
	if len(name) == 0 {
		return nil, errors.New("foundation name is required")
	}

	// Setup default configuration
	opts := &FoundationOptions{
		httpAddr:         config.LookupEnv("FOUNDATION_HTTP_ADDRESS", "0.0.0.0:8080"),
		grpcAddr:         config.LookupEnv("FOUNDATION_GRPC_ADDRESS", "0.0.0.0:8081"),
		httpWriteTimeout: 15 * time.Second,
		httpReadTimeout:  15 * time.Second,
		logger:           log.NewNop(),
	}
	for _, o := range options {
		o(opts)
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

// RegisterServiceFunc represents a function for registering a grpc service handler.
type RegisterServiceFunc func(s *grpc.Server)

// RegisterService registers a grpc service handler.
func (f *Foundation) RegisterService(fn RegisterServiceFunc) {
	// Create GRPC server only once
	f.grpcOnce.Do(func() {
		f.grpcServer = grpckit.NewServer(f.opts.grpcServerOpts...)
	})
	fn(f.grpcServer)
}

// initHTTPServerOnce will initialize the HTTP server once.
func (f *Foundation) initHTTPServerOnce() {
	f.httpOnce.Do(func() {
		opts := f.opts
		name := f.name

		// create http router
		r := mux.NewRouter()
		r.Use(handlers.CompressHandler)

		// Provide tracing for OTEL
		r.Use(otelmux.Middleware(name, otelmux.WithSpanNameFormatter(func(routeName string, r *http.Request) string {
			return fmt.Sprintf("[%s] %s", r.Method, r.RequestURI)
		})))

		// Provide Prometheus metric
		// The metrics measured are based on RED and/or Four golden signals,
		// follow standards and try to be measured in an efficient way.
		r.Use(telemetry.Middleware(middleware.New(middleware.Config{
			Service:  name,
			Recorder: metrics.NewRecorder(metrics.Config{}),
		})))

		r.StrictSlash(true)

		// If cors is enabled, we should set it depending on the options
		if opts.enableCors {
			r.Use(cors.New(opts.corsOpts).Handler)
		}

		f.httpRouter = r
		// create http server with options
		f.httpServer = &http.Server{
			Addr:         opts.httpAddr,
			Handler:      f.httpRouter,
			WriteTimeout: opts.httpWriteTimeout,
			ReadTimeout:  opts.httpReadTimeout,
		}
	})

}

// RegisterServiceHandlerFunc represents a function for registering a grpc gateway service handler.
type RegisterServiceHandlerFunc func(gw *runtime.ServeMux, conn *grpc.ClientConn)

// RegisterServiceHandler registers a grpc-gateway service handler.
func (f *Foundation) RegisterServiceHandler(fn RegisterServiceHandlerFunc, muxOpts ...runtime.ServeMuxOption) {
	// Make sure we have an HTTP server setup
	f.initHTTPServerOnce()
	// Only create one time the gateway and grpc client
	f.gwOnce.Do(func() {
		f.logger.Info(context.Background(), "initializing grpc-gateway")
		conn, err := grpckit.NewClient(f.opts.grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			f.logger.Error(context.Background(), "fail creating grpc client for grpc-gateway", log.Error(err))
		}

		f.gwClient = conn

		muxOpts = append(
			muxOpts,
			runtime.WithIncomingHeaderMatcher(func(s string) (string, bool) {
				// Allowing passing custom headers
				if strings.HasPrefix(s, "X-") {
					return s, true
				}
				return runtime.DefaultHeaderMatcher(s)
			}),
			runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.HTTPBodyMarshaler{
				Marshaler: &runtime.JSONPb{
					MarshalOptions: protojson.MarshalOptions{
						UseProtoNames:   true,
						EmitUnpopulated: true,
					},
				},
			}),
			runtime.WithMetadata(func(ctx context.Context, req *http.Request) metadata.MD {
				md := make(map[string]string)
				if _, ok := runtime.HTTPPathPattern(ctx); ok {
					md["pattern"] = req.RequestURI
				}
				md["method"] = req.Method

				queryParams, err := json.Marshal(req.URL.Query())
				if err != nil {
					queryParams = []byte{}
				}
				md["query-params"] = string(queryParams[:])

				return metadata.New(md)
			}),
		)
		f.gw = runtime.NewServeMux(muxOpts...)
	})

	fn(f.gw, f.gwClient)
}

// RegisterHTTPHandler registers a custom HTTP handler.
func (f *Foundation) RegisterHTTPHandler(path string, fn http.HandlerFunc, methods ...string) {
	// make sure the HTTP server has been initialized
	f.initHTTPServerOnce()
	f.httpRouter.HandleFunc(path, fn).Methods(methods...)
}

// RegisterLiveness register a liveness function for /healthz
//
// Many applications running for long periods of time eventually transition to broken states,
// and cannot recover except by being restarted.
// Kubernetes provides liveness probes to detect and remedy such situations.
func (f *Foundation) RegisterLiveness(fn func() (string, error)) {
	f.livenessProbe = handlerClosure(fn)
}

// RegisterReadiness register a readiness function for /readyz
//
// Sometimes, applications are temporarily unable to serve traffic.
// For example, an application might need to load a large amount of data or
// a large number of configuration files during startup.
// In such instances, we don’t want to kill the application, but we don’t want to send it requests either.
func (f *Foundation) RegisterReadiness(fn func() (string, error)) {
	f.readinessProbe = handlerClosure(fn)
}

func handlerClosure(fn func() (string, error)) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		status := http.StatusOK
		msg, err := fn()
		if err != nil {
			status = http.StatusInternalServerError
			msg = err.Error()
		}
		writer.WriteHeader(status)
		writer.Header().Set("Content-Type", "text/plain")
		fmt.Fprintln(writer, msg)
	}
}

// Serve configure and start serving request for the foundation service.
func (f *Foundation) Serve() error {
	_, err := maxprocs.Set(maxprocs.Logger(func(s string, i ...interface{}) {
		f.logger.Info(context.Background(), fmt.Sprintf(s, i))
	}))
	if err != nil {
		return errors.Wrap(err, "defining maxprocs")
	}

	// Setup telemetry
	tracer, err := telemetry.NewTracer(f.name)
	if err != nil {
		return errors.Wrap(err, "creating new tracer")
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tracer.Shutdown(ctx) //nolint
	}()

	_, err = telemetry.NewMeter(f.name)
	if err != nil {
		return errors.Wrap(err, "creating new meter")
	}

	// register health probes and profiling
	internalHTTP(f.logger, f.readinessProbe, f.livenessProbe)

	// shutdown channel to listen for an interrupt or terminate signal from the OS.
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Make a channel to listen for errors coming from the listener. Use a
	// buffered channel so the goroutine can exit if we don't collect this error.
	serverError := make(chan error, 1)

	// start the grpc server
	go func(serverError chan error) {
		// No GRPC server set up.
		if f.grpcServer == nil {
			return
		}

		// enable grpc metrics
		// This operation needs to be done after user register the proto to the server.
		grpcprometheus.EnableHandlingTimeHistogram()
		grpcprometheus.Register(f.grpcServer)

		// Create listener for the grpc server
		listen, err := net.Listen("tcp", f.opts.grpcAddr)
		if err != nil {
			serverError <- errors.Wrap(err, "init net listener")
		}
		serverError <- f.grpcServer.Serve(listen)
		_ = listen.Close() //nolint
	}(serverError)

	// start the http server
	go func(serverError chan error) {
		// No HTTP server set up.
		if f.httpServer == nil {
			return
		}

		// init the http server
		if f.gw != nil {
			f.httpRouter.PathPrefix("/").Handler(f.gw)
		}
		serverError <- f.httpServer.ListenAndServe()
	}(serverError)

	f.logger.Debug(context.Background(), "service started", log.String("service-name", f.name))

	select {
	case err := <-serverError:
		return errors.Wrap(err, "server error")
	case <-shutdown:

		// Terminate GRPC server if started
		if f.grpcServer != nil {
			f.grpcServer.GracefulStop()
		}

		// terminate the HTTP server if started.
		if f.httpServer != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_ = f.httpServer.Shutdown(ctx) //nolint
		}
	}

	return nil
}

// internalHTTP start a new http server for health checks and profiling.
func internalHTTP(l *log.Logger, readiness http.HandlerFunc, liveliness http.HandlerFunc) {

	r := mux.NewRouter()
	r.StrictSlash(true)

	// Init default health checks.
	r.HandleFunc("/healthz", liveliness).Name("healthz").Methods("GET")
	r.HandleFunc("/readyz", readiness).Name("readyz").Methods("GET")

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

	// create http server with options
	httpServer := http.Server{
		Addr:        ":9091",
		Handler:     r,
		ReadTimeout: 15 * time.Second,
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil {
			l.Debug(context.TODO(), "fail to start probe server", log.Error(err))
		}
	}()
}
