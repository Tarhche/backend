package gateway

import (
	"errors"
)

const (
	// defaultSubjectPrefix namespaces a gateway's subjects on the broker. It is
	// part of the wire contract: the workers that answer these requests consume
	// the prefixed subjects, so changing it means changing them too.
	defaultSubjectPrefix = "websocket_"

	// defaultRegistrySize is what one connection is expected to have in flight
	// at once.
	defaultRegistrySize = 8

	// defaultMaxInFlightRequests caps what one connection may leave unanswered,
	// so a client that never gets its replies cannot grow its registry without
	// bound for as long as it stays connected.
	defaultMaxInFlightRequests = 64

	// defaultReplyBuffer absorbs short bursts of replies without making the
	// fanout wait on a session.
	defaultReplyBuffer = 10
)

// configuration holds a Gateway's tunables. Every field has a working default.
type configuration struct {
	subjectPrefix       string
	replyBuffer         int
	maxInFlightRequests int
	replyBackoff        Backoff
	queueBackoff        Backoff
	registries          func() RequestRegistry
}

// Option overrides one of a Gateway's defaults.
type Option func(*configuration)

func newConfiguration(options ...Option) (configuration, error) {
	c := configuration{
		subjectPrefix:       defaultSubjectPrefix,
		replyBuffer:         defaultReplyBuffer,
		maxInFlightRequests: defaultMaxInFlightRequests,
		replyBackoff:        NewFixedBackoff(defaultReplyAttempts, defaultReplyWait),
		queueBackoff:        NewFixedBackoff(defaultQueueAttempts, defaultQueueWait),
		registries:          func() RequestRegistry { return NewInMemoryRequestRegistry(defaultRegistrySize) },
	}

	for _, option := range options {
		option(&c)
	}

	return c, c.validate()
}

func (c configuration) validate() error {
	if c.replyBuffer < 1 {
		return errors.New("reply buffer must be greater than zero, otherwise a reply only lands when a session happens to be waiting on it")
	}

	if c.maxInFlightRequests < 1 {
		return errors.New("max in-flight requests must be greater than zero, otherwise no request can be made")
	}

	if c.replyBackoff == nil {
		return errors.New("reply backoff cannot be nil")
	}

	if c.queueBackoff == nil {
		return errors.New("queue backoff cannot be nil")
	}

	if c.registries == nil {
		return errors.New("request registry factory cannot be nil")
	}

	return nil
}

// WithSubjectPrefix namespaces the subjects this gateway produces and consumes
// on. It has to match what the workers on the other side listen to.
func WithSubjectPrefix(prefix string) Option {
	return func(c *configuration) {
		c.subjectPrefix = prefix
	}
}

// WithReplyBuffer sets how many replies may queue up for one session before the
// fanout starts skipping it.
func WithReplyBuffer(size int) Option {
	return func(c *configuration) {
		c.replyBuffer = size
	}
}

// WithMaxInFlightRequests caps how many requests one connection may have
// waiting for a reply. Further requests are rejected until some are answered.
func WithMaxInFlightRequests(limit int) Option {
	return func(c *configuration) {
		c.maxInFlightRequests = limit
	}
}

// WithReplyBackoff sets how a reply is retried when the registry cannot say who
// it belongs to. The default makes three attempts, a second apart. Every
// connection shares the one given here, so it must be safe for concurrent use.
func WithReplyBackoff(backoff Backoff) Option {
	return func(c *configuration) {
		c.replyBackoff = backoff
	}
}

// WithQueueBackoff sets how a reply is retried when the client's queue is full.
// The default makes three attempts, 50ms apart. Every connection shares the one
// given here, so it must be safe for concurrent use.
func WithQueueBackoff(backoff Backoff) Option {
	return func(c *configuration) {
		c.queueBackoff = backoff
	}
}

// WithRequestRegistry replaces how a connection's request registry is built.
// The default gives every connection one of its own, held in memory: the
// request ids a client picks are private to it, and a session can only resolve
// a reply to a request it registered itself. A shared or remote registry gives
// up that guarantee, so it has to be asked for.
func WithRequestRegistry(registries func() RequestRegistry) Option {
	return func(c *configuration) {
		c.registries = registries
	}
}
