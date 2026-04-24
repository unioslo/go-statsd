package gostatsd

import (
	"fmt"
	"log/slog"
	"net"
	"sync"
)

const defaultBufferSize = 256

// Client is a StatsD UDP client. A nil *Client is safe to use; all methods become no-ops.
type Client struct {
	conn   *net.UDPConn
	prefix string
	logger *slog.Logger
	queue  chan string
	wg     sync.WaitGroup
}

// Option configures a Client.
type Option func(*clientConfig)

type clientConfig struct {
	prefix     string
	logger     *slog.Logger
	bufferSize int
}

// WithPrefix sets the metric name prefix prepended to every metric (e.g. "myapp.").
func WithPrefix(prefix string) Option {
	return func(c *clientConfig) { c.prefix = prefix }
}

// WithLogger sets the slog.Logger used for diagnostic messages. Nil means silent (the default).
func WithLogger(l *slog.Logger) Option {
	return func(c *clientConfig) { c.logger = l }
}

// WithBufferSize sets the depth of the internal send queue (default 256).
func WithBufferSize(n int) Option {
	return func(c *clientConfig) { c.bufferSize = n }
}

// New creates a StatsD client connected to the given UDP address.
// Returns an error immediately if the address cannot be resolved or the UDP connection cannot be opened.
// Increment and Middleware are safe to call on a nil *Client.
func New(addr string, opts ...Option) (*Client, error) {
	cfg := clientConfig{bufferSize: defaultBufferSize}
	for _, opt := range opts {
		opt(&cfg)
	}

	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("statsd: resolve address %q: %w", addr, err)
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return nil, fmt.Errorf("statsd: dial UDP %q: %w", addr, err)
	}

	c := &Client{
		conn:   conn,
		prefix: cfg.prefix,
		logger: cfg.logger,
		queue:  make(chan string, cfg.bufferSize),
	}

	c.wg.Add(1)
	go c.worker()

	return c, nil
}

func (c *Client) worker() {
	defer c.wg.Done()
	for payload := range c.queue {
		if _, err := c.conn.Write([]byte(payload)); err != nil && c.logger != nil {
			c.logger.Warn("statsd: failed to send metric", "error", err)
		}
	}
}

// Increment sends a counter increment for metric in StatsD wire format ("prefix.metric:1|c").
// Non-blocking: if the send queue is full the metric is dropped (logged at Warn if a logger is set).
// Safe to call on a nil *Client.
func (c *Client) Increment(metric string) {
	if c == nil {
		return
	}
	payload := fmt.Sprintf("%s%s:1|c", c.prefix, metric)
	select {
	case c.queue <- payload:
	default:
		if c.logger != nil {
			c.logger.Warn("statsd: send queue full, dropping metric", "metric", metric)
		}
	}
}

// Close drains the send queue, waits for the worker goroutine to finish, then closes the UDP connection.
func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	close(c.queue)
	c.wg.Wait()
	return c.conn.Close()
}
