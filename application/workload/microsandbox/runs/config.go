package runs

import "time"

// The supervisor's constants, as the plan fixes them. They are fields of
// Config rather than settings anyone can change: the provider fills Config from
// these, and only tests choose others, to run in milliseconds what takes
// seconds in production.
const (
	// DefaultMemoryFloor is the least memory a VM is given, whatever its run
	// asked for: the least a guest boots in.
	DefaultMemoryFloor = 64 << 20

	// DefaultOverhead is what admission counts for each VM on top of its
	// memory: what microsandbox itself needs on the host to run one. The
	// spike measured about 14 MiB for an idle VM beyond its guest's RAM,
	// page cache included, and this is twice that.
	DefaultOverhead = 32 << 20

	// DefaultMaxTCPConnections bounds the TCP connections one guest holds at
	// once, so a run cannot use up the host's.
	//
	// It bounds what the guest dials. What dials the guest is bounded
	// separately, and lower than one would like: microsandbox's port
	// publisher resets inbound connections beyond 256 per sandbox, and
	// counts a closed one for about ten seconds after it closed, so a busy
	// task's published port takes only about 256 new connections every ten
	// seconds. Nothing here can raise that.
	DefaultMaxTCPConnections = 256

	// DefaultConcurrentBoots is how many VMs boot at once. Booting is the
	// expensive part of a start; the rest waits its turn.
	DefaultConcurrentBoots = 4

	// DefaultStopGrace is how long a main process is given between its stop
	// signal and SIGKILL when nobody said, as docker's is.
	DefaultStopGrace = 10 * time.Second
)

// Config is how the supervisor runs. DefaultConfig is what the service runs
// with, apart from what its settings say: the budget, the floor, the bind
// address and the nameservers.
type Config struct {
	// Budget is the memory admitted for running VMs, in bytes. A run whose
	// VM would take the admitted total past it is refused with capacity.
	Budget uint64

	// MemoryFloor is the least memory a VM is given, and Overhead what
	// admission counts on top of each VM's memory.
	MemoryFloor uint64
	Overhead    uint64

	// BindAddress is where runs' published ports are bound on the host: an
	// address the orchestrators reach, since microsandbox binds 127.0.0.1
	// unless it is told otherwise.
	BindAddress string

	// Nameservers are the guests' DNS upstreams under the public policy.
	Nameservers []string

	MaxTCPConnections int
	ConcurrentBoots   int

	// StopGrace is the time between the stop signal and SIGKILL when a stop
	// does not say, and KillGrace the time a main process is given to end
	// after SIGKILL before its VM is stopped under it.
	StopGrace time.Duration
	KillGrace time.Duration

	// VMStopTimeout is how long a VM is given to shut down once its main
	// process has ended, after which microsandbox kills it.
	VMStopTimeout time.Duration

	// Backoff is how long the restart policy waits before each restart.
	Backoff Backoff

	// ExecEndGrace is how long an exec being ended is given to finish by
	// itself, and ExecKillGrace how long it is given after SIGTERM before
	// SIGKILL, as a container's exec is ended.
	ExecEndGrace  time.Duration
	ExecKillGrace time.Duration

	// MetricsTTL is how long one reading of every VM's metrics answers for,
	// so that many callers asking at once cost one call.
	MetricsTTL time.Duration

	// PullTimeout, QueueTimeout, BootTimeout and CallTimeout are the
	// supervisor's own deadlines: for pulling an image, for waiting for one
	// of the boot slots, for booting a VM and starting its main process, and
	// for any other call to microsandbox. They are the supervisor's rather
	// than a client's, because a call that a client gave up on is not
	// cancelled: cancelling microsandbox halfway through making a sandbox
	// leaves a stopped one behind.
	//
	// A boot takes about a second, and its main process starts within a
	// tenth of one; four booting at once on a busy host took six seconds at
	// worst. A queue of a whole node's runs coming back at once is what
	// QueueTimeout is for.
	PullTimeout  time.Duration
	QueueTimeout time.Duration
	BootTimeout  time.Duration
	CallTimeout  time.Duration

	// RetryInterval is how long the supervisor waits before checking the
	// runtime again, when it could not use it, and RetryMaxInterval the most
	// it ever waits.
	RetryInterval    time.Duration
	RetryMaxInterval time.Duration

	// ServiceVersion is the build of the service, which Info reports.
	ServiceVersion string
}

// Backoff is docker's restart backoff: a restart waits Initial, each one after
// it twice as long as the last up to Max, and a run that stayed up for
// ResetAfter starts again from Initial.
type Backoff struct {
	Initial    time.Duration
	Max        time.Duration
	ResetAfter time.Duration
}

// DefaultConfig is the supervisor's configuration with the plan's constants
// and no memory budget, which the service works out from its container's
// limit.
func DefaultConfig() Config {
	return Config{
		MemoryFloor:       DefaultMemoryFloor,
		Overhead:          DefaultOverhead,
		BindAddress:       "0.0.0.0",
		Nameservers:       []string{"1.1.1.1", "9.9.9.9"},
		MaxTCPConnections: DefaultMaxTCPConnections,
		ConcurrentBoots:   DefaultConcurrentBoots,
		StopGrace:         DefaultStopGrace,
		KillGrace:         5 * time.Second,
		VMStopTimeout:     5 * time.Second,
		Backoff: Backoff{
			Initial:    100 * time.Millisecond,
			Max:        time.Minute,
			ResetAfter: 10 * time.Second,
		},
		ExecEndGrace:     5 * time.Second,
		ExecKillGrace:    5 * time.Second,
		MetricsTTL:       time.Second,
		PullTimeout:      15 * time.Minute,
		QueueTimeout:     5 * time.Minute,
		BootTimeout:      time.Minute,
		CallTimeout:      30 * time.Second,
		RetryInterval:    time.Second,
		RetryMaxInterval: 30 * time.Second,
	}
}
