package websocket

import (
	"errors"
	"net/http"
	"time"
)

const (
	defaultMaxMessageSize = 1024 * 10 // 10kb
	defaultWriteWait      = 6 * time.Second
	defaultPingPeriod     = 2 * time.Second
	defaultPongWait       = 6 * time.Second

	// defaultOutboundBuffer absorbs short bursts without making the sender wait.
	defaultOutboundBuffer = 10
)

// configuration holds a Websocket's tunables. Every field has a working default.
type configuration struct {
	maxMessageSize int64
	writeWait      time.Duration
	pingPeriod     time.Duration
	pongWait       time.Duration
	outboundBuffer int
	checkOrigin    func(r *http.Request) bool
}

// Option overrides one of a Websocket's defaults.
type Option func(*configuration)

func newConfiguration(options ...Option) (configuration, error) {
	c := configuration{
		maxMessageSize: defaultMaxMessageSize,
		writeWait:      defaultWriteWait,
		pingPeriod:     defaultPingPeriod,
		pongWait:       defaultPongWait,
		outboundBuffer: defaultOutboundBuffer,
		checkOrigin:    func(*http.Request) bool { return true },
	}

	for _, option := range options {
		option(&c)
	}

	return c, c.validate()
}

func (c configuration) validate() error {
	if c.pingPeriod >= c.pongWait {
		return errors.New("ping period must be less than pong wait, otherwise a client is disconnected before it can answer a ping")
	}

	if c.maxMessageSize <= 0 {
		return errors.New("max message size must be greater than zero")
	}

	if c.writeWait <= 0 {
		return errors.New("write wait must be greater than zero")
	}

	if c.outboundBuffer < 0 {
		return errors.New("outbound buffer cannot be negative")
	}

	if c.checkOrigin == nil {
		return errors.New("origin checker cannot be nil")
	}

	return nil
}

// WithMaxMessageSize limits the size of a single client message.
func WithMaxMessageSize(size int64) Option {
	return func(c *configuration) {
		c.maxMessageSize = size
	}
}

// WithWriteWait sets how long a write to a client may take before it is abandoned.
func WithWriteWait(d time.Duration) Option {
	return func(c *configuration) {
		c.writeWait = d
	}
}

// WithPingPeriod sets how often clients are pinged. It must be less than the
// pong wait.
func WithPingPeriod(d time.Duration) Option {
	return func(c *configuration) {
		c.pingPeriod = d
	}
}

// WithPongWait sets how long a client may stay silent before it is considered gone.
func WithPongWait(d time.Duration) Option {
	return func(c *configuration) {
		c.pongWait = d
	}
}

// WithOutboundBuffer sets how many messages may queue up for a client before
// further ones are dropped.
func WithOutboundBuffer(size int) Option {
	return func(c *configuration) {
		c.outboundBuffer = size
	}
}

// WithOriginChecker replaces the origin check run before upgrading. The default
// accepts every origin.
func WithOriginChecker(check func(r *http.Request) bool) Option {
	return func(c *configuration) {
		c.checkOrigin = check
	}
}
