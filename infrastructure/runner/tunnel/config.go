package tunnel

import (
	"errors"
	"time"

	"github.com/xtaci/smux"
)

// The defaults, and why they are what they are.
//
// The numbers below are a memory budget before they are anything else. What one
// session can hold in flight is, in each direction:
//
//	streams × (MaxStreamBuffer + copyBufferSize)   bounded by MaxReceiveBuffer
//
// so the ceiling for a worker holding maxSessions at full stream capacity is
// maxSessions × MaxReceiveBuffer, plus the copy buffers. With the values here
// that is 4 × 4 MB = 16 MB of window per direction, and 256 × 4 × 32 KB = 32 MB
// of copy buffer if every stream is busy at once. Raising stream counts without
// lowering buffers is how a tunnel runs a machine out of memory.
const (
	// defaultVersion is smux 2. Version 2 is what gives a stream its own
	// receive window rather than sharing the session's, which is the whole
	// reason a slow target does not stall the streams beside it. Version 1 is
	// only for talking to something that cannot do 2.
	defaultVersion = 2

	// defaultKeepAliveInterval is how often an idle session sends a NOP. It has
	// to be well under the idle timeout of anything in between — a NAT table, a
	// cloud load balancer — which is commonly 60s and sometimes 30s. Ten
	// seconds gives three tries inside a 30s window.
	defaultKeepAliveInterval = 10 * time.Second

	// defaultKeepAliveTimeout is when a session with nothing arriving is
	// declared dead. It must be a multiple of the interval, or a single lost
	// NOP kills a healthy session; three intervals tolerates two losses. This
	// is also the worst case for noticing a worker that was unplugged rather
	// than closed, which is why the registry does not wait for it to decide a
	// worker is gone — a closed session is removed at once.
	defaultKeepAliveTimeout = 30 * time.Second

	// defaultMaxFrameSize is what one smux frame carries. It is kept under a
	// typical 64 KB socket buffer so a frame is not split across two reads, and
	// well above the ~1460 byte MSS so framing overhead is negligible. Larger
	// frames raise the head-of-line delay a big write inflicts on the streams
	// sharing the session, which is the thing to trade against.
	defaultMaxFrameSize = 32 * 1024

	// defaultMaxReceiveBuffer is the whole session's receive window: the
	// ceiling on what every stream on it can hold unread between them. It is
	// what stops one busy session from being unbounded.
	defaultMaxReceiveBuffer = 4 * 1024 * 1024

	// defaultMaxStreamBuffer is one stream's receive window, and so the
	// bandwidth-delay product a single stream can fill. At 10ms round trip it
	// allows about 50 Mbit/s on one stream; a fast, long path wants more, a
	// machine holding many streams wants less.
	defaultMaxStreamBuffer = 256 * 1024

	// defaultMinSessions is how many connections a worker keeps to each
	// ingress even when nothing is using them. Two rather than one so that
	// losing a connection does not leave the worker briefly unreachable while
	// it dials again.
	defaultMinSessions = 2

	// defaultMaxSessions bounds what one worker may open to one ingress. More
	// sessions is how throughput grows — each is its own TCP connection with
	// its own congestion window, and the only way past the head-of-line
	// blocking that a single connection imposes on every stream it carries.
	defaultMaxSessions = 4

	// defaultMaxStreamsPerSession is how many streams share one connection.
	// This is a blast radius before it is a capacity: every stream here dies
	// together when the connection does, and every one of them stalls together
	// when a packet on it is lost. It is high because the common load is many
	// mostly-idle connections; a throughput-shaped load wants it far lower and
	// maxSessions higher. Benchmarks in this package compare the two.
	defaultMaxStreamsPerSession = 256

	// defaultGrowThreshold is how full the pool gets before the worker opens
	// another session. Growing at capacity is too late: the ingress can only
	// wait, and cannot make capacity itself.
	defaultGrowThreshold = 0.75

	// defaultIdleSessionTimeout is how long a session above the minimum stays
	// open with nothing on it before the worker lets it go.
	defaultIdleSessionTimeout = 2 * time.Minute

	// defaultHandshakeTimeout bounds registering a connection and opening a
	// stream. Both are a round trip on a connection that already exists.
	defaultHandshakeTimeout = 10 * time.Second

	// defaultDialTimeout bounds dialling an ingress, and a worker dialling a
	// target.
	defaultDialTimeout = 10 * time.Second

	// defaultCapacityWait is how long a client waits when every session of the
	// worker it wants is full. The worker grows its pool on its own, so this is
	// waiting for that rather than for anything the ingress can do.
	defaultCapacityWait = 5 * time.Second

	// reconnect backoff. The delay doubles to the maximum and every wait is
	// jittered across its whole range, so a thousand workers that lost the same
	// ingress do not come back in step and knock it over again.
	defaultReconnectMinDelay = 500 * time.Millisecond
	defaultReconnectMaxDelay = 30 * time.Second

	// copyBufferSize is what one direction of one stream copies through. It
	// matches the frame size so a full buffer is one frame.
	copyBufferSize = 32 * 1024
)

var (
	ErrInvalidConfig = errors.New("tunnel: invalid configuration")
)

// Config is what both ends of a tunnel are built from. The zero value is not
// usable; start from DefaultConfig.
type Config struct {
	// Version, KeepAlive*, MaxFrameSize, MaxReceiveBuffer and MaxStreamBuffer
	// are smux's own, kept here so both ends are configured from one place and
	// so they can be tuned without reaching into smux.
	Version           int
	KeepAliveInterval time.Duration
	KeepAliveTimeout  time.Duration
	MaxFrameSize      int
	MaxReceiveBuffer  int
	MaxStreamBuffer   int

	// MinSessions and MaxSessions bound a worker's pool to one ingress.
	MinSessions int
	MaxSessions int

	// MaxStreamsPerSession is how many streams one session will carry before
	// the next one is used instead.
	MaxStreamsPerSession int

	// GrowThreshold is the fraction of the pool's stream capacity in use at
	// which a worker opens another session, between 0 and 1.
	GrowThreshold float64

	// IdleSessionTimeout is how long a session beyond MinSessions stays open
	// carrying nothing. Zero keeps them forever.
	IdleSessionTimeout time.Duration

	// HandshakeTimeout bounds registration and stream opening.
	HandshakeTimeout time.Duration

	// DialTimeout bounds dialling an ingress and dialling a target.
	DialTimeout time.Duration

	// CapacityWait is how long Dial waits for a worker to make room.
	CapacityWait time.Duration

	// ReconnectMinDelay and ReconnectMaxDelay bound the jittered backoff a
	// worker reconnects with.
	ReconnectMinDelay time.Duration
	ReconnectMaxDelay time.Duration
}

// DefaultConfig returns the configuration described at the top of this file.
func DefaultConfig() Config {
	return Config{
		Version:              defaultVersion,
		KeepAliveInterval:    defaultKeepAliveInterval,
		KeepAliveTimeout:     defaultKeepAliveTimeout,
		MaxFrameSize:         defaultMaxFrameSize,
		MaxReceiveBuffer:     defaultMaxReceiveBuffer,
		MaxStreamBuffer:      defaultMaxStreamBuffer,
		MinSessions:          defaultMinSessions,
		MaxSessions:          defaultMaxSessions,
		MaxStreamsPerSession: defaultMaxStreamsPerSession,
		GrowThreshold:        defaultGrowThreshold,
		IdleSessionTimeout:   defaultIdleSessionTimeout,
		HandshakeTimeout:     defaultHandshakeTimeout,
		DialTimeout:          defaultDialTimeout,
		CapacityWait:         defaultCapacityWait,
		ReconnectMinDelay:    defaultReconnectMinDelay,
		ReconnectMaxDelay:    defaultReconnectMaxDelay,
	}
}

// Validate reports a configuration that cannot work, rather than one that is
// merely unwise.
func (c Config) Validate() error {
	switch {
	case c.MinSessions < 1:
		return errWith("at least one session has to be kept")
	case c.MaxSessions < c.MinSessions:
		return errWith("the most sessions cannot be fewer than the fewest")
	case c.MaxStreamsPerSession < 1:
		return errWith("a session has to carry at least one stream")
	case c.GrowThreshold <= 0 || c.GrowThreshold > 1:
		return errWith("the threshold to grow at is a fraction above zero")
	case c.MaxStreamBuffer > c.MaxReceiveBuffer:
		return errWith("one stream cannot be allowed more than the whole session")
	case c.MaxFrameSize > c.MaxStreamBuffer:
		return errWith("a frame cannot be larger than the window it has to fit in")
	}

	return smux.VerifyConfig(c.smux())
}

// smux turns the settings smux owns back into its own configuration.
func (c Config) smux() *smux.Config {
	return &smux.Config{
		Version:           c.Version,
		KeepAliveInterval: c.KeepAliveInterval,
		KeepAliveTimeout:  c.KeepAliveTimeout,
		MaxFrameSize:      c.MaxFrameSize,
		MaxReceiveBuffer:  c.MaxReceiveBuffer,
		MaxStreamBuffer:   c.MaxStreamBuffer,
	}
}

// Capacity is how many streams a worker's pool can carry when it is at its
// largest.
func (c Config) Capacity() int {
	return c.MaxSessions * c.MaxStreamsPerSession
}

func errWith(reason string) error {
	return errors.Join(ErrInvalidConfig, errors.New(reason))
}
