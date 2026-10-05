package task

import (
	"context"
	"math"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Task represents a task specification
type Task struct {
	UUID string
	Name string
	Slug string
	Kind Kind

	// CurrentState is what the task is doing, as the node holding it last
	// reported. ExpectedState is what it was asked to be doing. The two drift
	// apart when a task stops, fails or is taken away behind the workload's
	// back, and closing that gap is what the control plane's own heartbeat does.
	CurrentState  State
	ExpectedState State

	// LastHeartbeatAt is when the node holding this task last said
	// anything about it. A task nobody has spoken for in a while is one
	// that is no longer there, whatever it was last seen doing.
	LastHeartbeatAt time.Time

	Image         string
	AutoRemove    bool
	PortBindings  []port.PortMap
	ExposedPorts  []port.Port
	NetworkPolicy network.Policy
	Endpoints     []Endpoint
	RestartPolicy string
	RestartCount  uint
	HealthCheck   string
	AttachStdin   bool
	AttachStdout  bool
	AttachStderr  bool
	Environment   []string
	Command       []string
	Entrypoint    []string
	WorkingDir    string

	// Interactive says this task is meant to be watched and reached while
	// it runs — a snippet on a page that serves something, or that somebody is
	// given a way into — rather than waited on for what it prints.
	Interactive bool

	// ReadOnly makes the task's own filesystem immutable: it may write
	// only to what is mounted into it. It is compose's read_only.
	ReadOnly bool

	// MaxRetries is how many times the workload asks for this task again
	// after it fails, before it gives up on making it what it was asked to be.
	// Zero is not at all, and RetryForever is for as long as it keeps failing.
	MaxRetries int

	// Retries is how many of those have happened since it last stood up. It is
	// counted in the messages between the workload's own parts, and written down
	// here only so that somebody looking at a task that keeps failing can
	// see what is being done about it.
	Retries int

	// Deadline is when the task running this task will be stopped for
	// having run long enough. The node that made the task sets it, so it
	// is counted from when the task came up rather than from when the
	// task was asked for.
	Deadline time.Time

	// TTL is how long a job may run for. A job that outlives it is stopped, and
	// then taken away like any other job that has ended. Zero is no limit, and
	// it means nothing for a service, which is meant to keep running.
	TTL time.Duration

	// Reason is why a task failed, when the workload can say: a task that
	// could never be created has nothing else to tell anybody, since it has no
	// log to read.
	Reason string

	Mounts         []Mount
	ResourceLimits ResourceLimits
	NodeName       string
	ExecutionID    string
	ExecutionLogs  []byte
	OwnerUUID      string
	CreatedAt      time.Time
	StartedAt      time.Time
	FinishedAt     time.Time
}

// Endpoint is an exposed task port that docker actually published, which
// is what makes it reachable at all. Where it was published is the node's own
// business and never leaves it, so what travels is only which ports came up.
type Endpoint struct {
	TaskPort port.Port
	HostPort port.Port
}

// Mount represents a mount point of volume
type Mount struct {
	Source   string
	Target   string
	Type     string
	ReadOnly bool
}

// ResourceLimits represents the resource limits of the task.
//
// Cpu is in cores. Memory and Disk are in bytes, and stay in bytes all the
// way down: a compose size like "256M" is turned into bytes when it is read,
// and nothing between there and the runtime converts it again.
type ResourceLimits struct {
	Cpu    float64
	Memory uint64
	Disk   uint64
}

// VMResources is what the VM a task runs in is given for these limits: its
// cores as whole vCPUs, rounded up and never to none, and its memory and disk
// as they are, in bytes.
func (l ResourceLimits) VMResources() vm.Resources {
	return vm.Resources{
		CPUs:   uint(max(math.Ceil(l.Cpu), 1)),
		Memory: l.Memory,
		Disk:   l.Disk,
	}
}

// GuestOwnerUUID is whose a task the code runner starts is: whoever is reading
// the page it was run from, signed in or not, which is nobody the users know.
// No user's uuid is ever the guest's, so nobody's own listing has one.
const GuestOwnerUUID = "guest"

// MinMemory is the least memory, in bytes, a task may be limited to.
//
// It is docker's floor rather than one the workload chose: the engine refuses
// to create a container whose memory limit is under 6 MiB, to leave room for
// what starting one costs. A task asking for less is told so when it is asked
// for, rather than accepted and then failed on whichever node it was given to.
const MinMemory = 6 << 20

// Repository represents a repository of tasks
type Repository interface {
	GetAll(ctx context.Context, offset uint, limit uint) ([]Task, error)

	// GetAllByOwner is the same listing, of what one person asked for. The
	// tasks the code runner started are the guest's (GuestOwnerUUID).
	GetAllByOwner(ctx context.Context, ownerUUID string, offset uint, limit uint) ([]Task, error)
	CountByOwner(ctx context.Context, ownerUUID string) (uint, error)

	GetOne(ctx context.Context, UUID string) (Task, error)

	// GetOneByOwner is the same, of one person's own. A task that is not
	// theirs is not there as far as they are concerned.
	GetOneByOwner(ctx context.Context, ownerUUID string, UUID string) (Task, error)
	GetOneBySlug(ctx context.Context, slug string) (Task, error)
	Save(ctx context.Context, t *Task) (uuid string, err error)
	Delete(ctx context.Context, UUID string) error
	Count(ctx context.Context) (uint, error)
}

type Scheduler interface {
	Pick(t *Task, candidates []node.Node) node.Node
}

// Removable reports whether nothing should be left of this task once it has
// ended.
//
// A job runs once and is done: what it was is its output, which has already
// been reported by the time it ends, so the task, its log and the task
// itself go together. A service is meant to keep running, and one that has
// stopped is still a task somebody can start again.
func (t *Task) Removable() bool {
	return IsTerminalState(t.CurrentState) && (t.Kind == KindJob || t.AutoRemove)
}

// OutlivedTTL reports whether a job has been running for longer than it was
// allowed to.
//
// It is measured from the moment the task started, so time spent waiting
// for a node does not count against what the job asked for. A task that has not
// started, has no limit, or is not a job, has nothing to outlive.
func (t *Task) OutlivedTTL(now time.Time) bool {
	if t.Kind != KindJob || t.TTL <= 0 || t.StartedAt.IsZero() {
		return false
	}

	return now.Sub(t.StartedAt) > t.TTL
}

// Silent reports whether nobody has spoken for this task in a while.
//
// A task is spoken for by the node holding it, over and over; one that has
// gone quiet was removed, or its node is gone, and either way what it was last
// seen doing is no longer what it is doing.
func (t *Task) Silent(now time.Time, after time.Duration) bool {
	last := t.LastHeartbeatAt

	// one nobody has ever spoken for has been quiet since it was asked for,
	// which is what a task whose node never took it looks like.
	if last.IsZero() {
		last = t.CreatedAt
	}

	if last.IsZero() {
		return false
	}

	return now.Sub(last) > after
}

// Drifted reports whether this task is not doing what it was asked to do.
//
// A task on its way somewhere — starting, stopping, restarting — has not
// drifted: it is on its way. Neither has one whose expectation was never set,
// which is every task from before there were expectations.
func (t *Task) Drifted(now time.Time, silentAfter time.Duration) bool {
	// asking for a task to be stopped is asking for it not to be running,
	// which a task that finished or failed already is not.
	if t.ExpectedState == Stopped && IsTerminalState(t.CurrentState) {
		return false
	}

	if t.ExpectedState == 0 || t.ExpectedState == t.CurrentState {
		// unless it has gone quiet while it was supposed to be running, in
		// which case what it is doing is nothing.
		return t.ExpectedState == Running && t.Silent(now, silentAfter)
	}

	if IsInFlightState(t.CurrentState) && !t.Silent(now, silentAfter) {
		return false
	}

	return true
}
