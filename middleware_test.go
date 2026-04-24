package gostatsd_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gostatsd "github.com/unioslo/go-statsd"
)

// newClientAndServer creates a test Client connected to a local UDP server.
// The caller is responsible for calling client.Close() — do NOT use t.Cleanup here
// because tests must call Close() explicitly to drain the queue before reading packets.
func newClientAndServer(t *testing.T) (*gostatsd.Client, *net.UDPConn) {
	t.Helper()
	server, addr := startUDPServer(t)
	client, err := gostatsd.New(addr)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return client, server
}

func TestMiddleware_2xx_IncrementsSuccess(t *testing.T) {
	client, server := newClientAndServer(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := client.Middleware(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	client.Close() // drain queue before reading

	got := readPacket(t, server)
	want := "server.request.success:1|c"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMiddleware_4xx_Increments4xx(t *testing.T) {
	client, server := newClientAndServer(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	wrapped := client.Middleware(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	client.Close()

	got := readPacket(t, server)
	want := "server.request.error.4xx:1|c"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMiddleware_5xx_Increments5xx(t *testing.T) {
	client, server := newClientAndServer(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	wrapped := client.Middleware(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	client.Close()

	got := readPacket(t, server)
	want := "server.request.error.5xx:1|c"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMiddleware_DefaultsTo200_IncrementsSuccess(t *testing.T) {
	client, server := newClientAndServer(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok")) // no WriteHeader call — should default to 200
	})
	wrapped := client.Middleware(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	client.Close()

	got := readPacket(t, server)
	want := "server.request.success:1|c"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMiddleware_IgnoredPath_SendsNoMetric(t *testing.T) {
	client, server := newClientAndServer(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped := client.Middleware(handler, "/.well-known/live", "/.well-known/ready", "/health")

	req := httptest.NewRequest("GET", "/.well-known/live", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)
	client.Close() // drain (nothing queued for ignored path)

	// No packet should arrive within 100ms.
	server.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	buf := make([]byte, 1024)
	n, _, err := server.ReadFromUDP(buf)
	if err == nil {
		t.Errorf("expected no metric for ignored path, got %q", string(buf[:n]))
	}
}

func TestMiddleware_NilClient_PassesThrough(t *testing.T) {
	var client *gostatsd.Client // nil — no statsd configured

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	wrapped := client.Middleware(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if !called {
		t.Error("expected handler to be called when client is nil")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}
