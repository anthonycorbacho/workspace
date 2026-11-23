package kit

import (
	"context"
	"net/http"
	"slices"
	"time"

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

	// Run application shutdown hooks in reverse registration order (LIFO) once inbound traffic has drained.
	f.hooksMu.Lock()
	hooks := slices.Clone(f.shutdownHooks)
	f.hooksMu.Unlock()
	for i := len(hooks) - 1; i >= 0; i-- {
		if err := hooks[i](ctx); err != nil {
			f.logger.Warn(ctx, "running shutdown hook", log.Error(err))
		}
	}

	// Keep readiness and metrics available throughout the public servers' drain and hooks.
	shutdownServers(ctx, f.logger, internalServers...)
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
