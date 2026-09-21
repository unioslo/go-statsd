package gostatsd

import (
	"context"
	"net/http"
	"slices"
)

type contextKey struct{}

type hint struct {
	skipErrorMetric bool
}

// SkipErrorMetric tells Middleware to count the current request as
// "server.request.success" regardless of its HTTP status code. Call it
// from a handler to suppress error counting for expected non-2xx responses
// (e.g. a form that is closed — a business outcome, not a server error).
// It is a no-op when called outside of a Middleware-wrapped handler.
func SkipErrorMetric(ctx context.Context) {
	if h, ok := ctx.Value(contextKey{}).(*hint); ok {
		h.skipErrorMetric = true
	}
}

// Middleware returns an HTTP handler that increments StatsD counters based on the response status code:
//   - 2xx–3xx → "server.request.success"
//   - 4xx     → "server.request.error.4xx"
//   - 5xx+    → "server.request.error.5xx"
//
// A handler may call SkipErrorMetric(r.Context()) to suppress error counting
// for an expected non-2xx response; the request is then counted as success.
//
// Requests whose path matches any of ignoredPaths are not counted.
// Safe to call on a nil *Client — returns next unchanged.
func (c *Client) Middleware(next http.Handler, ignoredPaths ...string) http.Handler {
	if c == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := &hint{}
		rw := &statusWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r.WithContext(context.WithValue(r.Context(), contextKey{}, h)))

		if slices.Contains(ignoredPaths, r.URL.Path) {
			return
		}

		if h.skipErrorMetric {
			c.Increment("server.request.success")
			return
		}

		// 1xx informational responses produce no metric.
		switch {
		case rw.statusCode >= 200 && rw.statusCode < 400:
			c.Increment("server.request.success")
		case rw.statusCode >= 400 && rw.statusCode < 500:
			c.Increment("server.request.error.4xx")
		case rw.statusCode >= 500:
			c.Increment("server.request.error.5xx")
		}
	})
}

type statusWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *statusWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}
