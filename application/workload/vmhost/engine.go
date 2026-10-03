// Package vmhost is what vmhost does with its VMs: it makes them, boots them,
// looks after them while they run, stops them, takes them back when it is
// started again, and lets go of them. The use cases beside it — one for each
// of vmhost's routes (domain/workload/vm/api.go) — validate what they are
// asked and hand it to the Engine here, which is the one place that knows how
// a VM's life goes, so that every route, and vmhost's own reconciling, go
// through it the same way.
//
// The engine is made of parts it knows only by their interfaces
// (domain/workload/vm): a Hypervisor that boots machines, an ImageStore that
// makes images into disks, a Fabric that networks machines, GuestClients of
// the agents inside them, and stores of their records and their output. It
// holds no process handles and no connections that matter beyond its own
// life: a VM's machine is the host's, and everything the engine knows about a
// VM is in its record, so a vmhost that is redeployed takes every VM back
// where the last one left it (Reconcile).
//
// A container runtime does a great deal for its containers that nobody else
// does for a microVM: it notices that a task ended, keeps what it wrote, and
// starts it again when its restart policy says so. For each running VM the
// engine keeps a keeper (keeper.go) that does exactly that, through the VM's
// agent. Ported from PR #101's runner, where it was proven end to end, and
// moved behind vmhost's API.
//
// What is done to one VM is done under that VM's own lock, so two requests for
// the same VM never interleave, and requests for different VMs never wait on
// each other. What reserves room — making a VM, booting one that had ended —
// is serialised besides, so two never take the same room.
package vmhost

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel/metric"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// lostExitCode is what a task is taken to have returned when its machine went
// away under it: as if it had been killed, which is what the 128+signal rule
// the orchestrator reads exit codes by says of 137.
const lostExitCode = 137

// Config is what the engine is told about the host it runs on and the budget
// it runs VMs in.
type Config struct {
	// Version is vmhost's own, which is the application's.
	Version string

	// ProcessMode is where machines' VMMs run, "systemd" or "child", as Info
	// reports it, and Architecture the host's, as Go names it.
	ProcessMode  string
	Architecture string

	// Kernel and Initrd are what every machine boots.
	Kernel string
	Initrd string

	// ScratchPath is where a VM's scratch disk is kept, which is in the VM's
	// own directory beside its record (layout.Scratch).
	ScratchPath func(id string) string

	// MaxMemory is the memory every VM together may be given, VMM overhead
	// included, and MemoryOverhead what each VM's VMM is counted as using
	// beside its guest's memory. MinMemory is the least a machine is given,
	// and MaxVMMemory the most one VM may ask for. In bytes.
	MaxMemory      uint64
	MinMemory      uint64
	MemoryOverhead uint64
	MaxVMMemory    uint64

	// MaxVMCPU is the most CPUs one VM may ask for. HostCPUs is how many the
	// host has, and CPUOvercommit how many of the VMs' CPUs there may be for
	// each of them; zero host CPUs counts nothing.
	MaxVMCPU      float64
	HostCPUs      int
	CPUOvercommit float64

	// DiskReserve is the free disk kept whatever VMs ask for, and
	// DiskOvercommit how many times the free disk their scratch disks may add
	// up to: they are sparse, so they rarely fill.
	DiskReserve    uint64
	DiskOvercommit float64

	// FirstUID and UIDs are the host users machines run as, one each; UIDs
	// zero runs every machine as vmhost itself, which is for development.
	FirstUID int
	UIDs     int

	// Nameservers are what a VM that routes out is given to ask for names.
	Nameservers []string

	// ImageCacheMax is how much disk images may take before the least
	// recently used that no VM boots are let go. Zero is no bound.
	ImageCacheMax uint64
}

// Parts are what the engine is made of.
type Parts struct {
	Hypervisor vm.Hypervisor
	Images     vm.ImageStore
	Fabric     vm.Fabric
	Guests     vm.GuestConnector
	States     vm.StateStore
	Logs       vm.LogStore

	// FreeSpace says how many bytes the data directory's filesystem has
	// left. Without it, disk is not admitted against.
	FreeSpace func() (uint64, error)

	// Usage reads what a running machine uses from the host's side. Without
	// it, a VM's stats are the guest's own view alone.
	Usage vm.UsageReader

	// Metrics is where the engine's measurements go. Without it, nowhere.
	Metrics metric.Meter
}

// timing is how long the engine gives things. It is the engine's own, rather
// than configuration, and only tests change it.
type timing struct {
	// boot is how long a machine has to boot and answer, and adopt how long
	// the agent of a machine being taken back has to answer.
	boot  time.Duration
	adopt time.Duration

	// settle is how long a machine is given to turn itself off before it is
	// ended from outside, and finalize bounds letting go of one.
	settle   time.Duration
	finalize time.Duration

	// retry is how long a keeper waits before asking an agent that did not
	// answer again, and gone how often a machine asked to turn off is looked
	// at until it is off.
	retry time.Duration
	gone  time.Duration

	// drain bounds reading what a task wrote last, once it ended, and poll
	// is how often a reader of a VM's output with nothing to wait on looks
	// again.
	drain time.Duration
	poll  time.Duration

	// imageGrace is how recently used an image may be and still be let go
	// of when the cache is full: one that was just made may be about to be
	// booted.
	imageGrace time.Duration

	// backoff is how long a task that has been started again n times waits
	// before the next.
	backoff func(n uint) time.Duration
}

func defaultTiming() timing {
	return timing{
		boot:       30 * time.Second,
		adopt:      10 * time.Second,
		settle:     5 * time.Second,
		finalize:   30 * time.Second,
		retry:      time.Second,
		gone:       100 * time.Millisecond,
		drain:      10 * time.Second,
		poll:       time.Second,
		imageGrace: 15 * time.Minute,
		backoff:    vm.Backoff,
	}
}

// Engine drives VMs through their lives.
type Engine struct {
	config Config

	hypervisor vm.Hypervisor
	images     vm.ImageStore
	fabric     vm.Fabric
	guests     vm.GuestConnector
	states     vm.StateStore
	logs       vm.LogStore
	freeSpace  func() (uint64, error)
	usage      vm.UsageReader

	logger  *slog.Logger
	metrics *instruments
	timing  timing

	// ctx lives as long as the engine does, and every keeper with it.
	// Ending it lets go of the VMs without ending any of them: they are the
	// host's, and the next vmhost takes them back.
	ctx    context.Context
	cancel context.CancelFunc

	// locks serialise what is done to one VM.
	locks keyedLock

	// admission serialises what reserves room: making a VM and booting one
	// that had ended, so that two never take the same room; and what checks
	// whether an image is booted with what lets go of it.
	admission sync.Mutex

	// plugging serialises giving a machine its taps, and writing them down,
	// with giving back the taps of machines nothing accounts for: one never
	// takes the other's for left over.
	plugging sync.Mutex

	// reconciling keeps to one reconcile at a time.
	reconciling sync.Mutex

	// background is what the engine started on its own and has to wait for
	// when it is closed: VMs deleted once their task ended, and reboots.
	background sync.WaitGroup

	lock sync.Mutex

	// keepers are the running VMs being looked after.
	keepers map[string]*keeper

	// restarting are the VMs being restarted: stopped only to be started
	// again, which is not an end anybody is to be told about.
	restarting map[string]bool

	// pending are the VMs whose machine went away, waiting to be booted
	// again by their restart policy, and rebootSeq counts the waits.
	pending   map[string]*pendingReboot
	rebootSeq uint64

	// samples are the last host readings of what each running machine used,
	// which its CPU is measured from at the next.
	samples map[string]sample

	healthy      bool
	healthReason string
	publicMade   bool
	closed       bool
}

// errMissingPart is an engine asked to be made without one of its parts.
var errMissingPart = errors.New("vmhost cannot run without its hypervisor, image store, fabric, guest connector, state store and log store")

// New makes an engine of its parts. It starts nothing: Reconcile takes back
// what an earlier vmhost left running, and is what makes the engine healthy.
func New(config Config, parts Parts, logger *slog.Logger) (*Engine, error) {
	if parts.Hypervisor == nil || parts.Images == nil || parts.Fabric == nil || parts.Guests == nil || parts.States == nil || parts.Logs == nil {
		return nil, errMissingPart
	}

	if config.ScratchPath == nil {
		return nil, errors.New("vmhost has to be told where scratch disks are kept")
	}

	ctx, cancel := context.WithCancel(context.Background())

	e := &Engine{
		config:       config,
		hypervisor:   parts.Hypervisor,
		images:       parts.Images,
		fabric:       parts.Fabric,
		guests:       parts.Guests,
		states:       parts.States,
		logs:         parts.Logs,
		freeSpace:    parts.FreeSpace,
		usage:        parts.Usage,
		logger:       logger,
		timing:       defaultTiming(),
		ctx:          ctx,
		cancel:       cancel,
		keepers:      make(map[string]*keeper),
		restarting:   make(map[string]bool),
		pending:      make(map[string]*pendingReboot),
		samples:      make(map[string]sample),
		healthReason: "vmhost has not looked at its machines yet",
	}

	metrics, err := newInstruments(parts.Metrics, e)
	if err != nil {
		cancel()

		return nil, err
	}

	e.metrics = metrics

	return e, nil
}

// Close lets go of every VM, leaving each of them as it is: running ones keep
// running, and the next vmhost takes them back. It waits for the keepers to
// let go until ctx is done.
func (e *Engine) Close(ctx context.Context) error {
	e.lock.Lock()
	e.closed = true

	for id, pending := range e.pending {
		pending.timer.Stop()
		delete(e.pending, id)
	}

	keepers := make([]*keeper, 0, len(e.keepers))
	for _, k := range e.keepers {
		keepers = append(keepers, k)
	}
	e.lock.Unlock()

	e.cancel()

	var err error

	for _, k := range keepers {
		select {
		case <-k.done:
			k.release()
		case <-ctx.Done():
			err = ctx.Err()
		}
	}

	finished := make(chan struct{})
	go func() {
		e.background.Wait()
		close(finished)
	}()

	select {
	case <-finished:
	case <-ctx.Done():
		err = ctx.Err()
	}

	// a hypervisor holding anything of its own — a connection to the host's
	// systemd — lets go of it last, once nothing asks it anything.
	var closed error
	if closer, ok := e.hypervisor.(interface{ Close() error }); ok {
		closed = closer.Close()
	}

	return errors.Join(err, closed, e.metrics.close())
}

// Health says whether vmhost can make and boot machines right now, and why
// not when it cannot. It is what the last Reconcile found.
func (e *Engine) Health() (bool, string) {
	e.lock.Lock()
	defer e.lock.Unlock()

	return e.healthy, e.healthReason
}

func (e *Engine) setHealth(healthy bool, reason string) {
	e.lock.Lock()
	defer e.lock.Unlock()

	e.healthy, e.healthReason = healthy, reason
}

// VM is one VM, or vm.ErrNotFound.
func (e *Engine) VM(ctx context.Context, id string) (vm.VM, error) {
	return e.states.Get(ctx, id)
}

// VMs is every VM carrying every label filter given, each key=value, oldest
// first.
func (e *Engine) VMs(ctx context.Context, filters []string) ([]vm.VM, error) {
	all, err := e.states.All(ctx)
	if err != nil {
		return nil, err
	}

	matching := make([]vm.VM, 0, len(all))
	for _, v := range all {
		if v.Matches(filters) {
			matching = append(matching, v)
		}
	}

	return matching, nil
}

// keeper is the keeper looking after a VM, if one is.
func (e *Engine) keeper(id string) *keeper {
	e.lock.Lock()
	defer e.lock.Unlock()

	return e.keepers[id]
}

// detach stops counting a keeper as the one looking after its VM.
func (e *Engine) detach(k *keeper) {
	e.lock.Lock()
	defer e.lock.Unlock()

	if e.keepers[k.id] == k {
		delete(e.keepers, k.id)
	}
}

func (e *Engine) markRestarting(id string, restarting bool) {
	e.lock.Lock()
	defer e.lock.Unlock()

	if restarting {
		e.restarting[id] = true
	} else {
		delete(e.restarting, id)
	}
}

func (e *Engine) isRestarting(id string) bool {
	e.lock.Lock()
	defer e.lock.Unlock()

	return e.restarting[id]
}

// isPending reports whether a VM is waiting to be booted again.
func (e *Engine) isPending(id string) bool {
	e.lock.Lock()
	defer e.lock.Unlock()

	return e.pending[id] != nil
}

// cancelReboot stops a VM waiting to be booted again from being booted, and
// reports whether it was waiting.
func (e *Engine) cancelReboot(id string) bool {
	e.lock.Lock()
	defer e.lock.Unlock()

	pending, found := e.pending[id]
	if found {
		pending.timer.Stop()
		delete(e.pending, id)
	}

	return found
}

// later runs work in the background, unless the engine is closing, and
// reports whether it will. The engine waits for it when it is closed.
func (e *Engine) later(work func()) bool {
	e.lock.Lock()
	defer e.lock.Unlock()

	if e.closed {
		return false
	}

	e.background.Add(1)

	go func() {
		defer e.background.Done()
		work()
	}()

	return true
}
