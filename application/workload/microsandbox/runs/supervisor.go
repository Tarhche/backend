// Package runs is the core of the workload-microsandbox service: the
// supervisor of every run the service holds.
//
// A run is one run of a task in a microVM, the service's counterpart of a
// container. Microsandbox has none of what a container runtime gives a
// container besides the VM itself: no main process that is the workload's own,
// no restart policy, no log that outlives its first command, no exit code for a
// process a signal ended, no stop that ends in a kill, and no memory budget.
// The supervisor is all of those, on top of a port, Sandboxes, that an adapter
// over microsandbox's SDK implements in a package only the service's image
// compiles. Everything here is static and tested against a fake of that port.
//
// # What a run is made of
//
// Each run has a record, kept by Records, which is everything the service needs
// to know about it after a restart; a journal of what its main process wrote,
// kept by Journal; and, while it runs, a live VM with its main process, which
// is the service's alone. The supervisor holds the main process's handle for
// exactly as long as the process runs, because closing it ends the process,
// and so does the service going away: microsandbox kills the commands of a
// client that disconnects, so no main process outlives the service that
// started it. Closing a sandbox's handle, by contrast, ends nothing, so a VM
// is always stopped explicitly before its handle is let go.
//
// Every start boots the run's VM, and every end of its main process stops it,
// so nothing of one run survives into the next but its disk, and memory is
// given back in between.
//
// # Concurrency
//
// The supervisor's maps and every run's state are guarded by one mutex, held
// only briefly and never across a call to microsandbox. What changes a run's
// life — start, stop, kill, restart, delete, and the restart policy's own
// restarts — is serialized by a lock of the run's own, which is held across
// those calls, so that two of them never act on one VM at once. Each live run
// has a keeper goroutine that reads its main process's events into the
// journal, and, once the process ends, stops the VM, records how the process
// ended, and applies the restart policy.
package runs

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

// The labels a run's sandbox carries. They exist only to match a sandbox to
// its run's record, and to find a sandbox that has none; microsandbox keeps
// the sandbox., microsandbox. and service. prefixes for itself.
const (
	LabelManaged = "workload.managed"
	LabelRun     = "workload.run"
	LabelNode    = "workload.node"
)

// sandboxPrefix starts every sandbox's name, which is the prefix and the run's
// ID.
const sandboxPrefix = "wk-"

// SandboxName is the name of a run's sandbox.
func SandboxName(id string) string {
	return sandboxPrefix + id
}

// Supervisor holds every run the service holds.
type Supervisor struct {
	sandboxes Sandboxes
	records   Records
	journal   Journal
	hostPorts HostPorts
	config    Config
	logger    *slog.Logger

	// boots is a slot for each VM that may boot at once.
	boots chan struct{}

	mu sync.Mutex

	runs  map[string]*run
	names map[nameKey]string

	// admitted is the memory admitted for the VMs that are up.
	admitted uint64

	// versions is what checking the runtime found. checked is whether it
	// has been checked, loaded whether the records have been read and
	// reconciled with the sandboxes that are there, and ready whether the
	// runs that were running have been started again.
	versions    Versions
	checked     bool
	recordsRead bool
	loaded      bool
	ready       bool

	// reason is why the service is not ready, while it is not.
	reason string

	// closing is a service that is shutting down, which starts nothing.
	closing bool

	// pulls are the pulls under way, by reference, so that many runs of one
	// image wait for one pull.
	pulls map[string]*pull

	metricsMu     sync.Mutex
	metricsAt     time.Time
	metricsValues map[string]Metrics

	// background is every goroutine the supervisor started for itself: the
	// keepers, the restart policy's timers, and the drains of execs.
	background sync.WaitGroup
}

type nameKey struct {
	node string
	name string
}

// run is one run, as the supervisor holds it.
type run struct {
	id string

	// op is the run's own lock, held across everything that changes its
	// life. It is a channel so that taking it can give up.
	op chan struct{}

	// saving serializes writing the run's record, so that an older state
	// can never be written over a newer one.
	saving sync.Mutex

	// What follows is guarded by Supervisor.mu.

	record Record

	// live is the run's VM and main process while it is up.
	live *live

	// pending is the restart policy's restart, waiting out its backoff.
	pending *pendingRestart

	// backoff is how long the policy's last restart waited.
	backoff time.Duration

	// execs are the commands started inside the run, by ID.
	execs map[string]*Exec

	// changed is closed, and replaced, whenever the run's state or its
	// journal changes, which is what wakes a log being followed.
	changed chan struct{}

	// lastAt is the stamp of the newest line in the run's journal, which
	// the next line has to come after. lastKnown is whether it has been
	// read from the journal since the service started.
	lastAt    time.Time
	lastKnown bool

	deleted bool
}

// live is a run's VM and its main process, while the process runs.
type live struct {
	sandbox    Sandbox
	process    Process
	stopSignal syscall.Signal

	// admitted is the memory admitted for the VM.
	admitted  uint64
	startedAt time.Time

	// What follows is guarded by Supervisor.mu.

	// sent is the last signal the supervisor sent the main process, and
	// zero while it has sent none.
	sent syscall.Signal

	// forced is a VM the supervisor stopped under a main process that would
	// not end.
	forced bool

	// shutdown is a main process stopped because the service is shutting
	// down.
	shutdown bool

	// ended is closed once the main process has ended, its VM has been
	// stopped and the run recorded as exited or restarting.
	ended chan struct{}
}

// pendingRestart is a restart the policy is waiting to make.
type pendingRestart struct {
	timer     *time.Timer
	cancelled bool
}

// New makes a supervisor. It holds nothing, and answers nothing but Info,
// until Open has adopted what the service held before.
func New(
	sandboxes Sandboxes,
	records Records,
	journal Journal,
	hostPorts HostPorts,
	config Config,
	logger *slog.Logger,
) *Supervisor {
	if config.ConcurrentBoots < 1 {
		config.ConcurrentBoots = 1
	}

	return &Supervisor{
		sandboxes: sandboxes,
		records:   records,
		journal:   journal,
		hostPorts: hostPorts,
		config:    config,
		logger:    logger,
		boots:     make(chan struct{}, config.ConcurrentBoots),
		runs:      make(map[string]*run),
		names:     make(map[nameKey]string),
		pulls:     make(map[string]*pull),
		reason:    "the runtime has not been checked yet",
	}
}

// Info is what the service says about itself.
func (s *Supervisor) Info() api.Info {
	s.mu.Lock()
	defer s.mu.Unlock()

	architecture := s.versions.Architecture
	if len(architecture) == 0 {
		architecture = runtime.GOARCH
	}

	info := api.Info{
		APIVersion:          api.Version,
		ServiceVersion:      s.config.ServiceVersion,
		MicrosandboxVersion: versionOf(s.versions.Runtime),
		Ready:               s.ready && !s.closing,
		Architecture:        architecture,
	}

	switch {
	case s.closing:
		info.Reason = "the service is shutting down"
	case !s.ready:
		info.Reason = s.reason
	}

	return info
}

// Ping is whether the service is ready, which is what its healthcheck asks:
// a service that cannot run anything is not healthy, however well it answers.
func (s *Supervisor) Ping(context.Context) error {
	if info := s.Info(); !info.Ready {
		return fmt.Errorf("not ready: %s", info.Reason)
	}

	return nil
}

// List is the runs of a node, narrowed to one task or one slug when either is
// given, oldest first.
func (s *Supervisor) List(node, taskUUID, slug string) ([]api.Run, error) {
	if len(node) == 0 {
		return nil, newError(api.CodeInvalid, "node is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.readable(); err != nil {
		return nil, err
	}

	listed := make([]api.Run, 0)

	for _, r := range s.runs {
		spec := r.record.Spec

		if spec.Node != node {
			continue
		}

		if len(taskUUID) > 0 && spec.Task.UUID != taskUUID {
			continue
		}

		if len(slug) > 0 && spec.Task.Slug != slug {
			continue
		}

		listed = append(listed, r.record.Run())
	}

	slices.SortFunc(listed, func(a, b api.Run) int {
		if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
			return c
		}

		return strings.Compare(a.ID, b.ID)
	})

	return listed, nil
}

// Get is one run.
func (s *Supervisor) Get(id string) (api.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.readable(); err != nil {
		return api.Run{}, err
	}

	r, found := s.runs[id]
	if !found {
		return api.Run{}, notFound(id)
	}

	return r.record.Run(), nil
}

// Create records a run as it was asked for. It boots nothing, as docker create
// starts nothing: Start does.
func (s *Supervisor) Create(ctx context.Context, spec api.RunSpec) (api.Run, error) {
	if err := validateSpec(spec); err != nil {
		return api.Run{}, err
	}

	id, err := newID()
	if err != nil {
		return api.Run{}, err
	}

	r := &run{
		id: id,
		op: make(chan struct{}, 1),
		record: Record{
			ID:        id,
			Spec:      cloneSpec(spec),
			State:     api.StateCreated,
			CreatedAt: now(),
		},
		execs:   make(map[string]*Exec),
		changed: make(chan struct{}),
	}

	key := nameKey{node: spec.Node, name: spec.Name}

	s.mu.Lock()

	if err := s.startable(); err != nil {
		s.mu.Unlock()

		return api.Run{}, err
	}

	if existing, used := s.names[key]; used {
		s.mu.Unlock()

		return api.Run{}, newError(api.CodeNameInUse, "node %s already has a run named %s: %s", spec.Node, spec.Name, existing)
	}

	s.runs[id] = r
	s.names[key] = id
	view := r.record.Run()

	s.mu.Unlock()

	if err := s.save(r); err != nil {
		s.mu.Lock()
		delete(s.runs, id)
		delete(s.names, key)
		r.deleted = true
		s.mu.Unlock()

		return api.Run{}, fmt.Errorf("the run could not be recorded: %w", err)
	}

	return view, nil
}

// readable is whether runs can be read: once the records have been read and
// reconciled with the sandboxes that are there. Before that, a listing would
// say a node holds nothing, which the control plane would act on. Called
// with mu held.
func (s *Supervisor) readable() error {
	if !s.loaded {
		return newError(api.CodeUnavailable, "the service is not ready: %s", s.reason)
	}

	return nil
}

// changeable is whether a run's life can be changed: once the service is
// ready. Called with mu held.
func (s *Supervisor) changeable() error {
	if !s.ready {
		return newError(api.CodeUnavailable, "the service is not ready: %s", s.reason)
	}

	return nil
}

// startable is whether a run may be created or started: once the service is
// ready, and until it begins shutting down. Called with mu held.
func (s *Supervisor) startable() error {
	if s.closing {
		return newError(api.CodeUnavailable, "the service is shutting down")
	}

	return s.changeable()
}

// find is a run that can be changed. Called without mu held.
func (s *Supervisor) find(id string, starting bool) (*run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	check := s.changeable
	if starting {
		check = s.startable
	}

	if err := check(); err != nil {
		return nil, err
	}

	r, found := s.runs[id]
	if !found {
		return nil, notFound(id)
	}

	return r, nil
}

// lock takes a run's own lock, giving up after the longest any one change to
// a run should take, and hands back what releases it.
func (s *Supervisor) lock(r *run) (func(), error) {
	wait := time.NewTimer(s.config.QueueTimeout + s.config.BootTimeout + 2*s.config.CallTimeout)
	defer wait.Stop()

	select {
	case r.op <- struct{}{}:
		return func() { <-r.op }, nil
	case <-wait.C:
		return nil, fmt.Errorf("run %s is still busy with what was asked of it before", r.id)
	}
}

// save writes a run's record as it is now.
//
// It reads the record under mu and writes it outside, so a slow disk holds up
// only the run being written. Writes of one run take turns, and each writes
// the record as it is when its turn comes, so the last write is always of the
// newest state.
func (s *Supervisor) save(r *run) error {
	r.saving.Lock()
	defer r.saving.Unlock()

	s.mu.Lock()
	if r.deleted {
		s.mu.Unlock()

		return nil
	}
	record := r.record.clone()
	s.mu.Unlock()

	return s.records.Save(record)
}

// persist saves a run's record where nobody is waiting to hear that it failed:
// the run carries on as it is, and the next save tries again.
func (s *Supervisor) persist(r *run) {
	if err := s.save(r); err != nil {
		s.logger.Error("a run's record could not be saved", "run", r.id, "error", err)
	}
}

// notify wakes whoever is following the run's log. Called with mu held.
func (s *Supervisor) notify(r *run) {
	close(r.changed)
	r.changed = make(chan struct{})
}

// view is a run as a client sees it.
func (s *Supervisor) view(r *run) api.Run {
	s.mu.Lock()
	defer s.mu.Unlock()

	return r.record.Run()
}

func newID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("an ID could not be made: %w", err)
	}

	return hex.EncodeToString(id.Bytes()), nil
}

// now is the time without its monotonic reading, in UTC, which is how it is
// written down and compared once it has been.
func now() time.Time {
	return time.Now().UTC()
}

// labels are what a run's sandbox is labelled with.
func labels(id, node string) map[string]string {
	return map[string]string{
		LabelManaged: "true",
		LabelRun:     id,
		LabelNode:    node,
	}
}
