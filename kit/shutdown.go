package kit

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/anthonycorbacho/workspace/kit/errors"
	"github.com/anthonycorbacho/workspace/kit/log"
	"google.golang.org/grpc"
)

// handleReadiness keeps the service unready while its requests are draining, even with a custom probe.
func (f *Foundation) handleReadiness(w http.ResponseWriter, r *http.Request) {
	if f.draining.Load() {
		http.Error(w, "shutting down", http.StatusServiceUnavailable)
		return
	}
	f.readinessProbe(w, r)
}

// shutdown drains the public servers, runs shutdown hooks, and closes the probes and metrics servers.
func (f *Foundation) shutdown(internalServers ...*http.Server) {
	f.drain(true, internalServers...)
}

// drain marks the service unready, optionally waits for shutdownDelay, then drains the public servers,
// runs registered shutdown hooks in LIFO order, and closes the internal servers within the shared deadline.
func (f *Foundation) drain(waitDelay bool, internalServers ...*http.Server) {
	f.draining.Store(true)
	if waitDelay && f.opts.shutdownDelay > 0 {
		f.logger.Info(context.Background(), "waiting before draining traffic", log.Duration("delay", f.opts.shutdownDelay))
		time.Sleep(f.opts.shutdownDelay)
	}

	ctx, cancel := context.WithTimeout(context.Background(), f.opts.shutdownTimeout)
	defer cancel()

	// Stop accepting HTTP requests first. The gRPC server and gateway client must stay
	// available until the accepted HTTP requests finish forwarding their calls.
	if f.httpServer != nil {
		shutdownServers(ctx, f.logger, f.httpServer)
	}
	if f.gwClient != nil {
		if err := f.gwClient.Close(); err != nil {
			f.logger.Warn(ctx, "closing grpc-gateway client", log.Error(err))
		}
	}
	if f.grpcServer != nil {
		shutdownGRPC(ctx, f.logger, f.grpcServer)
	}

	f.runShutdownHooks(ctx)
	// Keep readiness and metrics available throughout the public servers' drain and hooks.
	shutdownServers(ctx, f.logger, internalServers...)
}

func (f *Foundation) runShutdownHooks(ctx context.Context) {
	f.hooksMu.Lock()
	hooks := slices.Clone(f.shutdownHooks)
	f.hooksMu.Unlock()

	// Only one hook runs at a time. It can still report its result if we stop waiting.
	result := make(chan error, 1)
	for i, hook := range slices.Backward(hooks) {
		if err := ctx.Err(); err != nil {
			f.logger.Warn(ctx, "skipping shutdown hooks", log.Int("remaining", i+1), log.Error(err))
			return
		}
		go func() { result <- callShutdownHook(ctx, hook) }()
		select {
		case err := <-result:
			if err != nil {
				f.logger.Warn(ctx, "running shutdown hook", log.Error(err))
			}
		case <-ctx.Done():
			f.logger.Warn(ctx, "shutdown hook exceeded grace period", log.Int("skipped", i), log.Error(ctx.Err()))
			return
		}
	}
}

// A hook runs in its own goroutine; contain its panic there, even if shutdown has returned.
func callShutdownHook(ctx context.Context, hook func(context.Context) error) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			err = errors.Newf("shutdown hook panicked: %v", p)
		}
	}()
	return hook(ctx)
}

// shutdownGRPC allows pending RPCs to finish until ctx is done, then forces their connections closed.
func shutdownGRPC(ctx context.Context, l *log.Logger, srv *grpc.Server) {
	stopped := make(chan struct{})
	go func() {
		srv.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-ctx.Done():
		l.Warn(ctx, "forcing grpc server shutdown", log.Error(ctx.Err()))
		// Stop can also wait for handlers (eg: grpc.WaitForHandlers), so neither it nor
		// the GracefulStop goroutine may hold up shutdown after the deadline.
		go srv.Stop()
	}
}

// shutdownServers gracefully shuts down the given HTTP servers, closing active connections on timeout.
func shutdownServers(ctx context.Context, l *log.Logger, servers ...*http.Server) {
	for _, srv := range servers {
		if err := srv.Shutdown(ctx); err != nil {
			l.Warn(ctx, "shutting down server", log.String("addr", srv.Addr), log.Error(err))
			if err := srv.Close(); err != nil {
				l.Warn(ctx, "forcing http server shutdown", log.String("addr", srv.Addr), log.Error(err))
			}
		}
	}
}
