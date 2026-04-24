package gostatsd

import (
	"net/http"
	"slices"
)

// Middleware returns an HTTP handler that increments StatsD counters based on the response status code:
//   - 2xx–3xx → "server.request.success"
//   - 4xx     → "server.request.error.4xx"
//   - 5xx+    → "server.request.error.5xx"
//
// Requests whose path matches any of ignoredPaths are not counted.
// Safe to call on a nil *Client — returns next unchanged.
func (c *Client) Middleware(next http.Handler, ignoredPaths ...string) http.Handler {
	if c == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &statusWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r)

		if slices.Contains(ignoredPaths, r.URL.Path) {
			return
		}

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
