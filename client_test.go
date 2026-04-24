package gostatsd_test

import (
	"net"
	"testing"
	"time"

	gostatsd "github.com/unioslo/go-statsd"
)

// startUDPServer opens a random-port UDP listener and registers cleanup.
// Shared by client_test.go and middleware_test.go (same package).
func startUDPServer(t *testing.T) (*net.UDPConn, string) {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("startUDPServer: %v", err)
	}
	t.Cleanup(func() { pc.Close() })
	return pc.(*net.UDPConn), pc.LocalAddr().String()
}

// readPacket reads one UDP datagram with a 1-second deadline.
func readPacket(t *testing.T, conn *net.UDPConn) string {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	buf := make([]byte, 1024)
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("readPacket: %v", err)
	}
	return string(buf[:n])
}

func TestNew_InvalidAddress(t *testing.T) {
	_, err := gostatsd.New("not-a-valid:::addr")
	if err == nil {
		t.Fatal("expected error for invalid address, got nil")
	}
}

func TestIncrement_SendsCorrectPayload(t *testing.T) {
	server, addr := startUDPServer(t)

	client, err := gostatsd.New(addr)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	client.Increment("test.metric")
	client.Close()

	got := readPacket(t, server)
	want := "test.metric:1|c"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestIncrement_WithPrefix(t *testing.T) {
	server, addr := startUDPServer(t)

	client, err := gostatsd.New(addr, gostatsd.WithPrefix("myapp."))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	client.Increment("requests")
	client.Close()

	got := readPacket(t, server)
	want := "myapp.requests:1|c"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestIncrement_NilClient_DoesNotPanic(t *testing.T) {
	var client *gostatsd.Client
	client.Increment("test.metric") // must not panic
}

func TestIncrement_DoesNotBlockWhenQueueFull(t *testing.T) {
	_, addr := startUDPServer(t)

	client, err := gostatsd.New(addr, gostatsd.WithBufferSize(1))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer client.Close()

	// Send many metrics — some will be dropped, none should block or panic.
	for i := 0; i < 1000; i++ {
		client.Increment("test.metric")
	}
}

func TestClose_DrainsAllQueued(t *testing.T) {
	server, addr := startUDPServer(t)

	client, err := gostatsd.New(addr, gostatsd.WithBufferSize(100))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	const n = 10
	for i := 0; i < n; i++ {
		client.Increment("test.metric")
	}
	client.Close() // must wait until all n metrics are sent

	received := 0
	if err := server.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	buf := make([]byte, 1024)
	for {
		_, _, err := server.ReadFromUDP(buf)
		if err != nil {
			break // deadline = no more packets
		}
		received++
	}
	if received != n {
		t.Errorf("expected %d metrics drained, got %d", n, received)
	}
}
