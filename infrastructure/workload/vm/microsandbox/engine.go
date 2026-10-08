//go:build microsandbox

package microsandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	msb "github.com/superradcompany/microsandbox/sdk/go"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// stopTimeout is how long a VM is given to stop on its own: a Docker VM's
	// vminit gives dockerd 30 s, and dockerd gives each container 10 s.
	stopTimeout = 60 * time.Second

	// killTimeout bounds waiting for a VM that was killed to be seen stopped.
	killTimeout = 15 * time.Second

	// preStopTimeout bounds stopping a Docker VM's supervisor, which waits up
	// to 45 s for dockerd.
	preStopTimeout = 50 * time.Second

	// endedStopTimeout is how long a VM whose main process has ended is given
	// to stop: nothing in it is left to stop gracefully.
	endedStopTimeout = 10 * time.Second

	// dockerReadyTimeout is how long a Docker VM's boot holds its turn while
	// dockerd comes up. dockerd's start is what a boot storm is made of, so
	// the next boot waits for it; one that is not up by then is left to come
	// up while the others boot.
	dockerReadyTimeout = 2 * time.Minute
	dockerReadyPoll    = 500 * time.Millisecond

	// commandTimeout bounds one of the engine's own commands in a VM.
	commandTimeout = 30 * time.Second

	// adoptTimeout bounds taking back one VM a vmhost before this one left
	// running.
	adoptTimeout = time.Minute

	// defaultConcurrentBoots is how many VMs boot at once when the options do
	// not say.
	defaultConcurrentBoots = 4

	// concurrentLogReads bounds how many logs are read at once: microsandbox
	// reads a whole log into a buffer of 48 MiB for each.
	concurrentLogReads = 4

	// listPageSize is the most sandboxes microsandbox lists at once.
	listPageSize = 100

	// snapshotPrefix names the snapshots the engine installs for as long as
	// one operation takes, so that what a vmhost that died left behind is
	// found and removed.
	snapshotPrefix = "vmhost-"
)

// engine is a vm.Engine on microsandbox.
//
// Each instance is a sandbox named by its id, and a record of the engine's
// own beside it. microsandbox runs the sandbox and says whether it is
// running; the record says everything microsandbox does not keep, or loses
// on a restore.
//
// Changes to one instance are made one at a time, in turn: its ops is held
// for as long as one takes, which may be minutes for a Docker VM. Reading an
// instance never waits for that, so a node says what it holds while it is
// changing it.
type engine struct {
	options Options
	logger  *slog.Logger

	store *store
	tmp   string

	ports        portRange
	budget       vm.Resources
	orchestrator string
	dockerImage  string

	boots    chan struct{}
	logReads chan struct{}

	// database holds microsandbox's database open for as long as the engine
	// runs (holdDatabase).
	database io.Closer

	lock      sync.Mutex
	instances map[string]*instance
	closing   bool
}

var _ Engine = &engine{}

// instance is one of the engine's instances.
type instance struct {
	id string

	// ops is held by whoever is changing the instance.
	ops chan struct{}

	mu sync.Mutex

	record *record

	// sandbox is a live handle into its running sandbox, when there is one,
	// which commands are exec'd through.
	sandbox *msb.Sandbox

	sessions map[*session]struct{}
	main     *mainProcess
	disk     diskSample
}

func newInstance(id string, r *record) *instance {
	return &instance{
		id:       id,
		ops:      make(chan struct{}, 1),
		record:   r,
		sessions: make(map[*session]struct{}),
	}
}

// acquire waits for the instance's turn to be changed.
func (i *instance) acquire(ctx context.Context) error {
	select {
	case i.ops <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (i *instance) release() {
	<-i.ops
}

// current is a copy of the instance's record.
func (i *instance) current() *record {
	i.mu.Lock()
	defer i.mu.Unlock()

	return i.record.clone()
}

// New is the engine of the vmhost it runs in. It takes back every instance a
// vmhost before it left, and boots nothing that is not running: what should
// be running is its orchestrator's to ask for.
func New(ctx context.Context, options Options) (Engine, error) {
	if options.Logger == nil {
		options.Logger = slog.New(slog.DiscardHandler)
	}

	if len(options.Home) == 0 || !filepath.IsAbs(options.Home) {
		return nil, fmt.Errorf("the engine's home is an absolute path, not %q", options.Home)
	}

	if _, err := netip.ParseAddr(options.BindAddress); err != nil {
		return nil, fmt.Errorf("the address published ports are bound to is an IP: %w", err)
	}

	orchestrator, err := addressRange(options.OrchestratorAddress)
	if err != nil {
		return nil, err
	}

	ports, err := newPortRange(options.FirstPort, options.LastPort)
	if err != nil {
		return nil, err
	}

	// microsandbox takes its home from the environment alone, when it is
	// first used, and the msb processes it starts take it from there too.
	if err := os.Setenv("MSB_HOME", options.Home); err != nil {
		return nil, err
	}

	home := filepath.Join(options.Home, "vmhost")

	records, err := newStore(filepath.Join(home, "instances"))
	if err != nil {
		return nil, err
	}

	// what an operation left in the middle of being written is of no use to
	// anybody now.
	tmp := filepath.Join(home, "tmp")
	if err := os.RemoveAll(tmp); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return nil, err
	}

	boots := options.MaxConcurrentBoots
	if boots == 0 {
		boots = defaultConcurrentBoots
	}

	e := &engine{
		options:      options,
		logger:       options.Logger,
		store:        records,
		tmp:          tmp,
		ports:        ports,
		budget:       budgetOf(options.Capacity, hostOf(options.Home)),
		orchestrator: orchestrator,
		dockerImage:  options.DockerImage,
		boots:        make(chan struct{}, boots),
		logReads:     make(chan struct{}, concurrentLogReads),
		instances:    make(map[string]*instance),
	}

	if err := e.adopt(ctx); err != nil {
		return nil, err
	}

	// taking back what was left has had microsandbox open its database, which
	// no msb process is to take itself for the last one to close from now on.
	database, err := holdDatabase(filepath.Join(options.Home, "db", "msb.db"))
	if err != nil {
		e.logger.Warn("the engine's database could not be held open: once a vm stops, the vmhost may be unable to read it until it restarts", "error", err)
	} else {
		e.database = database
	}

	e.logger.Info("vm engine ready",
		"engine", archiveEngine(),
		"instances", len(e.instances),
		"cpus", e.budget.CPUs,
		"memory", e.budget.Memory,
		"disk", e.budget.Disk,
	)

	// nothing the engine asks of microsandbox changes how a VM's memory is
	// backed: the host decides, and a host that leaves it to 4 KiB pages
	// says so where whoever runs it looks.
	if mode := hostHugePages(); mode != hugePagesAlways {
		e.logger.Warn("transparent huge pages are not always on: a vm's memory is faulted in 4 KiB at a time, which nested virtualization makes slow enough for a code runner's Go snippet to run out of time",
			"transparent_hugepage", mode,
			"path", hugePagesPath,
		)
	}

	return e, nil
}

// addressRange is the only address published ports take connections from,
// as a rule's destination: an IP, which is that address alone, or a range.
func addressRange(address string) (string, error) {
	if prefix, err := netip.ParsePrefix(address); err == nil {
		return prefix.Masked().String(), nil
	}

	ip, err := netip.ParseAddr(address)
	if err != nil {
		return "", fmt.Errorf("the orchestrator's address is an IP or a range of them, not %q", address)
	}

	return netip.PrefixFrom(ip, ip.BitLen()).String(), nil
}

// adopt takes back what a vmhost before this one left.
//
// An instance whose sandbox was never made is let go of. One whose main
// process was running lost it with that vmhost, since its exec session went
// with it, so it has ended without saying how and its VM is stopped. A
// running Docker VM is ensured, which brings dockerd's supervisor back to a
// VM that was restored. Nothing that is not running is started.
func (e *engine) adopt(ctx context.Context) error {
	records, err := e.store.load()
	if err != nil {
		return err
	}

	statuses, err := e.statuses(ctx)
	if err != nil {
		return fmt.Errorf("microsandbox cannot list its sandboxes: %w", err)
	}

	var adopting sync.WaitGroup

	for id, r := range records {
		status, exists := statuses[id]

		if r.Creating && !exists {
			e.logger.Warn("a vm whose create never finished is let go of", "vm", id)

			if err := e.store.remove(id); err != nil {
				return err
			}

			continue
		}

		i := newInstance(id, r)
		e.instances[id] = i

		// no main process outlives the vmhost that ran it, so one that was
		// running is gone, and so is one whose instance was being booted.
		lost := r.Spec.HasMainProcess() && r.Exit == nil && (r.MainRunning || up(status))

		if r.Creating || lost {
			if err := e.update(i, func(r *record) {
				r.Creating = false

				if lost {
					r.MainRunning = false
					r.Exit = &exit{Code: -1, Reason: "its main process ended with the vmhost that ran it", At: time.Now()}
				}
			}); err != nil {
				return err
			}
		}

		if !up(status) {
			continue
		}

		adopting.Go(func() {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), adoptTimeout)
			defer cancel()

			e.adoptRunning(ctx, i, lost)
		})
	}

	adopting.Wait()

	for id := range statuses {
		if _, ok := e.instances[id]; !ok {
			e.logger.Warn("a sandbox this engine has no record of is left alone", "sandbox", id)
		}
	}

	e.removeLeftoverSnapshots(ctx)

	return nil
}

// adoptRunning takes back one running instance.
func (e *engine) adoptRunning(ctx context.Context, i *instance, lost bool) {
	h, err := e.sandboxOf(ctx, i.id)
	if err != nil || h == nil {
		e.logger.Warn("a running vm could not be taken back", "vm", i.id, "error", err)

		return
	}

	if lost {
		e.logger.Warn("a vm whose main process ended with the last vmhost is stopped", "vm", i.id)

		if err := e.shutdownSandbox(ctx, i, h, endedStopTimeout); err != nil {
			e.logger.Warn("a vm whose main process ended could not be stopped", "vm", i.id, "error", err)
		}

		return
	}

	if i.current().Spec.Kind != vm.KindDocker {
		return
	}

	sb, err := e.live(ctx, i)
	if err != nil {
		e.logger.Warn("a running docker vm could not be reached", "vm", i.id, "error", err)

		return
	}

	if err := e.ensure(ctx, i, sb); err != nil {
		e.logger.Warn("dockerd's supervisor could not be ensured", "vm", i.id, "error", err)
	}
}

// removeLeftoverSnapshots removes the snapshots operations installed for as
// long as they took, which a vmhost that died during one left behind.
func (e *engine) removeLeftoverSnapshots(ctx context.Context) {
	snapshots, err := msb.Snapshot.List(ctx)
	if err != nil {
		e.logger.Warn("snapshots could not be listed", "error", err)

		return
	}

	for _, snapshot := range snapshots {
		name := snapshot.Name()
		if name == nil || !strings.HasPrefix(*name, snapshotPrefix) {
			continue
		}

		if err := snapshot.Remove(ctx, true); err != nil {
			e.logger.Warn("a leftover snapshot could not be removed", "snapshot", *name, "error", err)
		}
	}
}

func (e *engine) Info(ctx context.Context) (vm.Info, error) {
	e.lock.Lock()
	allocated := e.allocated("")
	e.lock.Unlock()

	return vm.Info{
		Engine:    Name,
		Version:   Version,
		CPUs:      e.budget.CPUs,
		Memory:    e.budget.Memory,
		Disk:      e.budget.Disk,
		Allocated: allocated,
	}, nil
}

func (e *engine) List(ctx context.Context) ([]vm.Instance, error) {
	statuses, err := e.statuses(ctx)
	if err != nil {
		return nil, err
	}

	e.lock.Lock()
	ids := slices.Sorted(maps.Keys(e.instances))
	held := make([]*instance, len(ids))
	for n, id := range ids {
		held[n] = e.instances[id]
	}
	e.lock.Unlock()

	instances := make([]vm.Instance, len(held))
	for n, i := range held {
		status, exists := statuses[i.id]
		instances[n] = e.view(i, status, exists)
	}

	return instances, nil
}

func (e *engine) Inspect(ctx context.Context, id string) (vm.Instance, error) {
	i, err := e.get(id)
	if err != nil {
		return vm.Instance{}, err
	}

	return e.inspect(ctx, i)
}

func (e *engine) inspect(ctx context.Context, i *instance) (vm.Instance, error) {
	h, err := e.sandboxOf(ctx, i.id)
	if err != nil {
		return vm.Instance{}, err
	}

	if h == nil {
		return e.view(i, "", false), nil
	}

	return e.view(i, h.Status(), true), nil
}

// view is an instance as the engine reports it: what its record says has
// become of it, and otherwise what its sandbox is doing.
func (e *engine) view(i *instance, status msb.SandboxStatus, exists bool) vm.Instance {
	r := i.current()

	instance := vm.Instance{
		ID:        i.id,
		Labels:    maps.Clone(r.Spec.Labels),
		StartedAt: r.StartedAt,
	}

	switch {
	case r.Exit != nil:
		instance.State = vm.InstanceExited
		instance.ExitCode = r.Exit.Code
		instance.Reason = r.Exit.Reason
	case len(r.Failure) > 0:
		instance.State = vm.InstanceFailed
		instance.Reason = r.Failure
	case !exists && r.Creating:
		instance.State = vm.InstanceCreated
	case !exists:
		instance.State = vm.InstanceFailed
		instance.Reason = "its sandbox is gone, and its disk with it"
	default:
		instance.State, instance.Reason = stateOf(status)
	}

	// nothing reaches an instance whose ingress is denied, so nothing of it
	// is published.
	if r.Spec.Network.Ingress != vm.AccessAllow {
		return instance
	}

	for _, guest := range slices.Sorted(maps.Keys(r.HostPorts)) {
		instance.Endpoints = append(instance.Endpoints, vm.Endpoint{
			Port:    guest,
			Address: net.JoinHostPort(e.options.BindAddress, strconv.FormatUint(uint64(r.HostPorts[guest]), 10)),
		})
	}

	return instance
}

// stateOf is what a sandbox's status is as an instance's state. One that
// crashed is down with its disk where it was, which is a stopped VM that did
// not stop by itself: a container restart leaves every VM that way.
func stateOf(status msb.SandboxStatus) (vm.InstanceState, string) {
	switch status {
	case msb.SandboxStatusRunning, msb.SandboxStatusDraining, msb.SandboxStatusPaused:
		return vm.InstanceRunning, ""
	case msb.SandboxStatusCreated, msb.SandboxStatusStarting:
		return vm.InstanceCreated, ""
	case msb.SandboxStatusCrashed:
		return vm.InstanceStopped, "it went down without being stopped"
	default:
		return vm.InstanceStopped, ""
	}
}

// up reports whether a sandbox is running, or on its way up.
func up(status msb.SandboxStatus) bool {
	switch status {
	case msb.SandboxStatusRunning, msb.SandboxStatusStarting, msb.SandboxStatusDraining, msb.SandboxStatusPaused:
		return true
	default:
		return false
	}
}

// get is the instance named id.
func (e *engine) get(id string) (*instance, error) {
	e.lock.Lock()
	defer e.lock.Unlock()

	i, ok := e.instances[id]
	if !ok {
		return nil, fmt.Errorf("%w: no instance %q", domain.ErrNotExists, id)
	}

	return i, nil
}

// sandboxOf is the sandbox named id, or nil when there is none.
func (e *engine) sandboxOf(ctx context.Context, id string) (*msb.SandboxHandle, error) {
	h, err := msb.GetSandbox(ctx, id)
	if msb.IsKind(err, msb.ErrSandboxNotFound) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return h, nil
}

// statuses is what every sandbox there is is doing, by name.
func (e *engine) statuses(ctx context.Context) (map[string]msb.SandboxStatus, error) {
	statuses := make(map[string]msb.SandboxStatus)
	options := []msb.SandboxListOption{msb.WithListLimit(listPageSize)}

	for {
		page, err := msb.ListSandboxesWith(ctx, options...)
		if err != nil {
			return nil, err
		}

		for _, h := range page.Sandboxes {
			statuses[h.Name()] = h.Status()
		}

		if page.NextCursor == nil || len(*page.NextCursor) == 0 || len(page.Sandboxes) == 0 {
			return statuses, nil
		}

		options = []msb.SandboxListOption{msb.WithListLimit(listPageSize), msb.WithListCursor(*page.NextCursor)}
	}
}

// live is a live handle into an instance's running sandbox, connected to when
// there is none yet. An instance that is not running has none.
func (e *engine) live(ctx context.Context, i *instance) (*msb.Sandbox, error) {
	h, err := e.sandboxOf(ctx, i.id)
	if err != nil {
		return nil, err
	}

	if h == nil || h.Status() != msb.SandboxStatusRunning {
		e.forget(i)

		status := "not there"
		if h != nil {
			status = string(h.Status())
		}

		return nil, fmt.Errorf("%w: %q is %s", vm.ErrNotRunning, i.id, status)
	}

	i.mu.Lock()
	sb := i.sandbox
	i.mu.Unlock()

	if sb != nil {
		return sb, nil
	}

	connected, err := h.Connect(ctx)
	if err != nil {
		return nil, err
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	// somebody connected meanwhile: theirs is kept, and this one let go of.
	if i.sandbox != nil {
		go func() { _ = connected.Detach(context.Background()) }()

		return i.sandbox, nil
	}

	i.sandbox = connected

	return connected, nil
}

// keep makes sb the live handle into an instance's sandbox, letting go of
// any it had.
func (e *engine) keep(i *instance, sb *msb.Sandbox) {
	i.mu.Lock()
	previous := i.sandbox
	i.sandbox = sb
	i.mu.Unlock()

	if previous != nil && previous != sb {
		go func() { _ = previous.Detach(context.Background()) }()
	}
}

// forget lets go of the live handle into an instance's sandbox, which leads
// nowhere once the sandbox is down.
func (e *engine) forget(i *instance) {
	e.keep(i, nil)
}

// update changes an instance's record and writes it down. The record changes
// only once it is written.
func (e *engine) update(i *instance, change func(r *record)) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	next := i.record.clone()
	change(next)

	if err := e.store.save(i.id, next); err != nil {
		return fmt.Errorf("the record of %q cannot be written: %w", i.id, err)
	}

	i.record = next

	return nil
}

// allocated is what every instance but the one named except was given. It is
// called with the engine's lock held.
func (e *engine) allocated(except string) vm.Resources {
	var total vm.Resources

	for id, i := range e.instances {
		if id == except {
			continue
		}

		i.mu.Lock()
		given := i.record.Spec.Resources
		i.mu.Unlock()

		total.CPUs += given.CPUs
		total.Memory += given.Memory
		total.Disk += given.Disk
	}

	return total
}

// hostPorts gives the guest ports of the instance named id host ports,
// keeping those it had. It is called with the engine's lock held.
func (e *engine) hostPorts(id string, guests []port.Port, had map[port.Port]port.Port) (map[port.Port]port.Port, error) {
	taken := make(map[port.Port]bool)

	for other, i := range e.instances {
		if other == id {
			continue
		}

		i.mu.Lock()
		for _, host := range i.record.HostPorts {
			taken[host] = true
		}
		i.mu.Unlock()
	}

	return e.ports.assign(guests, had, taken, e.bindable)
}

// bindable reports whether nothing in the container listens on a host port
// already, which something that is not the engine's may.
func (e *engine) bindable(p port.Port) bool {
	listener, err := net.Listen("tcp", net.JoinHostPort(e.options.BindAddress, strconv.FormatUint(uint64(p), 10)))
	if err != nil {
		return false
	}

	_ = listener.Close()

	return true
}

// shuttingDown refuses to boot anything once the engine is going away.
func (e *engine) shuttingDown() error {
	e.lock.Lock()
	defer e.lock.Unlock()

	if e.closing {
		return errors.New("the vm engine is shutting down")
	}

	return nil
}
