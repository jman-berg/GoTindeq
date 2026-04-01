package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"expvar"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os/signal"
	"runtime"
	"runtime/debug"
	"syscall"
	"time"
)

type Server struct {
	mux     *http.ServeMux
	version string
	logger  *slog.Logger
}

func NewServer(version string, w io.Writer) *Server {

	slog.SetDefault(slog.New(slog.NewJSONHandler(w, nil)))

	s := &Server{
		mux:     http.NewServeMux(),
		version: version,
		logger:  slog.Default(),
	}
	s.routes()

	return s
}

func (s *Server) Run(ctx context.Context, addr string) error {
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)

	server := &http.Server{
		Addr:              addr,
		Handler:           s.handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errChan := make(chan error, 1)
	go func() {
		slog.InfoContext(ctx, "server started")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		slog.InfoContext(ctx, "shutting down server")

		ctx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		// Shutdown the HTTP server first
		if err := server.Shutdown(ctx); err != nil {
			return fmt.Errorf("server shutdown: %w", err)
		}

		// After server is shutdown, cancel the main context to close other resources
		cancel()

		// Add cleanup code here, in reverse order of initialization
		// Give each cleanup operation its own timeout if needed

		// Example cleanup sequence:
		// 1. Close application services that depend on other resources
		// if err := myService.Shutdown(ctx); err != nil {
		//     return fmt.Errorf("service shutdown: %w", err)
		// }

		// 2. Close message queue connections
		// if err := mqClient.Close(); err != nil {
		//     return fmt.Errorf("mq shutdown: %w", err)
		// }

		// 3. Close cache connections
		// if err := cacheClient.Close(); err != nil {
		//     return fmt.Errorf("cache shutdown: %w", err)
		// }

		// 4. Close database connections
		// if err := db.Close(); err != nil {
		//     return fmt.Errorf("database shutdown: %w", err)
		// }
		return nil
	}
}

func (s *Server) handler() http.Handler {
	var h http.Handler = s.mux
	h = s.accesslog(h)
	h = s.recovery(h)
	return h
}

func (s *Server) handleGetHealth() http.HandlerFunc {
	type responseBody struct {
		Version        string    `json:"Version"`
		Uptime         string    `json:"Uptime"`
		LastCommitHash string    `json:"LastCommitHash"`
		LastCommitTime time.Time `json:"LastCommitTime"`
		DirtyBuild     bool      `json:"DirtyBuild"`
	}

	baseRes := responseBody{Version: s.version}
	buildInfo, _ := debug.ReadBuildInfo()
	for _, kv := range buildInfo.Settings {
		if kv.Value == "" {
			continue
		}
		switch kv.Key {
		case "vcs.revision":
			baseRes.LastCommitHash = kv.Value
		case "vcs.time":
			baseRes.LastCommitTime, _ = time.Parse(time.RFC3339, kv.Value)
		case "vcs.modified":
			baseRes.DirtyBuild = kv.Value == "true"
		}
	}

	up := time.Now()
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		res := baseRes // Create a copy for each request to avoid data race
		res.Uptime = time.Since(up).String()
		if err := json.NewEncoder(w).Encode(res); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// handleGetDebug returns an [http.Handler] for debug routes, including pprof and expvar routes.
func (s *Server) handleGetDebug() http.Handler {
	mux := http.NewServeMux()

	// NOTE: this route is same as defined in net/http/pprof init function
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// NOTE: this route is same as defined in expvar init function
	mux.Handle("/debug/vars", expvar.Handler())
	return mux
}

func (s *Server) accesslog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wr := responseRecorder{ResponseWriter: w}

		next.ServeHTTP(&wr, r)

		s.logger.InfoContext(r.Context(), "accessed",
			slog.String("latency", time.Since(start).String()),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("query", r.URL.RawQuery),
			slog.String("ip", r.RemoteAddr),
			slog.Int("status", wr.status),
			slog.Int("bytes", wr.numBytes))
	})
}

// recovery is a middleware that recovers from panics during HTTP handler execution and logs the error details.
// It must be the last middleware in the chain to ensure it captures all panics.
func (s *Server) recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wr := responseRecorder{ResponseWriter: w}
		defer func() {
			err := recover()
			if err == nil {
				return
			}

			if err, ok := err.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				// Handle the abort gracefully
				return
			}

			stack := make([]byte, 1024)
			n := runtime.Stack(stack, true)

			s.logger.ErrorContext(r.Context(), "panic!",
				slog.Any("error", err),
				slog.String("stack", string(stack[:n])),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("query", r.URL.RawQuery),
				slog.String("ip", r.RemoteAddr))

			if wr.status > 0 {
				// response was already sent, nothing we can do
				return
			}

			// send error response
			http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
		}()
		next.ServeHTTP(&wr, r)
	})
}

// responseRecorder is a wrapper around [http.ResponseWriter] that records the status and bytes written during the response.
// It implements the [http.ResponseWriter] interface by embedding the original ResponseWriter.
type responseRecorder struct {
	http.ResponseWriter
	status   int
	numBytes int
}

// Header implements the [http.ResponseWriter] interface.
func (re *responseRecorder) Header() http.Header {
	return re.ResponseWriter.Header()
}

// Write implements the [http.ResponseWriter] interface.
func (re *responseRecorder) Write(b []byte) (int, error) {
	re.numBytes += len(b)
	return re.ResponseWriter.Write(b)
}

// WriteHeader implements the [http.ResponseWriter] interface.
func (re *responseRecorder) WriteHeader(statusCode int) {
	re.status = statusCode
	re.ResponseWriter.WriteHeader(statusCode)
}
