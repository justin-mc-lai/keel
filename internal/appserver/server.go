// Package appserver owns the lifecycle of the HTTP shell: listen, serve, and
// shut down within a deadline when the context is cancelled.
package appserver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

// DefaultShutdownGrace is how long in-flight requests get after a signal.
const DefaultShutdownGrace = 10 * time.Second

// Run blocks until ctx is cancelled or the listener fails.
//
// It returns nil for a clean shutdown so a signal is not reported as an error.
func Run(ctx context.Context, addr string, handler http.Handler, grace time.Duration) error {
	if grace <= 0 {
		grace = DefaultShutdownGrace
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}

	errCh := make(chan error, 1)
	go func() {
		err := srv.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errCh <- err
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), grace)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		<-errCh
		return nil
	case err := <-errCh:
		return err
	}
}

// Addr lets tests discover the bound port.
func Listen(addr string) (net.Listener, error) { return net.Listen("tcp", addr) }
