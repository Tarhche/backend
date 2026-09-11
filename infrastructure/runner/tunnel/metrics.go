package tunnel

import "sync/atomic"

// Metrics is what the tunnel reports about itself.
//
// Every method is called on the data path, so an implementation has to be cheap
// and must not block. Nothing here carries a stream's contents — only who it
// belonged to and how much of it there was.
type Metrics interface {
	// SessionOpened and SessionClosed bracket one of a worker's connections.
	SessionOpened(worker string, session string)
	SessionClosed(worker string, session string, reason string)

	// StreamOpened and StreamClosed bracket one client connection. Bytes are
	// reported once, at the end, rather than as they go: counting every read
	// would put an atomic in the middle of the copy.
	StreamOpened(worker string, session string, target string)
	StreamClosed(worker string, session string, target string, sent int64, received int64)

	// StreamFailed is a stream that never carried anything, because no worker
	// could take it or the target could not be reached.
	StreamFailed(worker string, target string, reason string)

	// AuthenticationFailed is a connection that did not get in.
	AuthenticationFailed(worker string, reason string)

	// Reconnected is a worker's own count of having had to dial again.
	Reconnected(worker string, address string, attempt int)
}

// Counters is a Metrics that keeps totals in memory, which is enough to assert
// on in a test and enough to scrape in a small deployment.
type Counters struct {
	SessionsOpened atomic.Int64
	SessionsClosed atomic.Int64
	StreamsOpened  atomic.Int64
	StreamsClosed  atomic.Int64
	StreamsFailed  atomic.Int64
	AuthFailures   atomic.Int64
	Reconnects     atomic.Int64
	BytesSent      atomic.Int64
	BytesReceived  atomic.Int64
}

var _ Metrics = &Counters{}

func (c *Counters) SessionOpened(string, string)         { c.SessionsOpened.Add(1) }
func (c *Counters) SessionClosed(string, string, string) { c.SessionsClosed.Add(1) }
func (c *Counters) StreamOpened(string, string, string)  { c.StreamsOpened.Add(1) }

func (c *Counters) StreamClosed(_ string, _ string, _ string, sent int64, received int64) {
	c.StreamsClosed.Add(1)
	c.BytesSent.Add(sent)
	c.BytesReceived.Add(received)
}

func (c *Counters) StreamFailed(string, string, string) { c.StreamsFailed.Add(1) }
func (c *Counters) AuthenticationFailed(string, string) { c.AuthFailures.Add(1) }
func (c *Counters) Reconnected(string, string, int)     { c.Reconnects.Add(1) }

// discardMetrics is what is used when nothing was given.
type discardMetrics struct{}

var _ Metrics = discardMetrics{}

func (discardMetrics) SessionOpened(string, string)                      {}
func (discardMetrics) SessionClosed(string, string, string)              {}
func (discardMetrics) StreamOpened(string, string, string)               {}
func (discardMetrics) StreamClosed(string, string, string, int64, int64) {}
func (discardMetrics) StreamFailed(string, string, string)               {}
func (discardMetrics) AuthenticationFailed(string, string)               {}
func (discardMetrics) Reconnected(string, string, int)                   {}
