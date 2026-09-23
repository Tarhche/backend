// Package firecracker runs an orchestrator's tasks in microVMs: a machine each,
// booted by firecracker, behind the task.Runtime everything above it asks.
//
// Read the README beside this file first; it carries the whole design.
package firecracker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/machine"
	"github.com/khanzadimahdi/testproject/domain/runner/network"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/guest"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/image"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/layout"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

const (
	// vsockName is where a machine's firecracker exposes its vsock, inside
	// the machine's own directory.
	vsockName = "v.sock"

	// bootTimeout is how long a machine has to boot and answer, and
	// stopTimeout how long its task has to end on its own when stopped.
	bootTimeout = 30 * time.Second
	stopTimeout = 10 * time.Second

	// settleTimeout is how long a machine is given to turn itself off before
	// it is ended from outside.
	settleTimeout = 5 * time.Second

	// finalizeTimeout bounds letting go of a machine whose task has ended.
	finalizeTimeout = 30 * time.Second

	// lostExitCode is what a task is taken to have returned when its machine
	// went away under it, as if it had been killed.
	lostExitCode = 137
)

// errNoMachine is a machine this orchestrator does not hold.
var errNoMachine = fmt.Errorf("%w: no such machine", domain.ErrNotExists)

// Config is what an orchestrator's machines are made of.
type Config struct {
	// Owner is the orchestrator: every machine it makes is its own.
	Owner string

	// StateDir is shared with the launcher at the same path on both sides.
	StateDir string

	// Kernel and Initrd are what every machine boots: the one kernel, and
	// the initramfs holding the agent.
	Kernel string
	Initrd string

	// Nameservers are given to a machine that routes out.
	Nameservers []string

	// UID and GID are whose what the orchestrator makes for machines is.
	UID int
	GID int
}

// Runtime runs an orchestrator's tasks in microVMs.
type Runtime struct {
	config   Config
	launcher machine.Launcher
	images   *image.Store
	networks *Networks
	store    *store
	logger   *slog.Logger
	tracer   oteltrace.Tracer

	// ctx lives as long as the runtime does, and every machine's keeper with
	// it. Ending it lets go of the machines without ending any of them.
	ctx    context.Context
	cancel context.CancelFunc

	locks keyedLock

	lock    sync.Mutex
	keepers map[string]*keeper
}

var _ task.Runtime = &Runtime{}

// New makes a runtime of what an orchestrator kept the last time it ran, and
// takes back the machines still running from then.
func New(config Config, launcher machine.Launcher, logger *slog.Logger) (*Runtime, *Networks, error) {
	images, err := image.NewStore(layout.Images(config.StateDir), config.UID, config.GID, logger)
	if err != nil {
		return nil, nil, err
	}

	machines := layout.Machines(config.StateDir, config.Owner)

	records, err := openStore(machines)
	if err != nil {
		return nil, nil, err
	}

	networks, err := NewNetworks(launcher, config.Owner, filepath.Dir(machines))
	if err != nil {
		return nil, nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	r := &Runtime{
		config:   config,
		launcher: launcher,
		images:   images,
		networks: networks,
		store:    records,
		logger:   logger,
		tracer:   otel.Tracer("firecracker"),
		ctx:      ctx,
		cancel:   cancel,
		keepers:  make(map[string]*keeper),
	}

	// the launcher may not be up yet — it restarts, or it comes up after the
	// orchestrator does — which is no reason for the orchestrator not to.
	go r.reconcileUntilDone()

	return r, networks, nil
}

// Close lets go of the machines, leaving every one of them running: they are
// the host's, and the next orchestrator to run takes them back.
func (r *Runtime) Close(ctx context.Context) error {
	r.cancel()

	r.lock.Lock()
	keepers := make([]*keeper, 0, len(r.keepers))
	for _, k := range r.keepers {
		keepers = append(keepers, k)
	}
	r.lock.Unlock()

	for _, k := range keepers {
		select {
		case <-k.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

func (r *Runtime) OnNode(ctx context.Context, nodeName string) ([]task.Execution, error) {
	return r.matching(func(rec record) bool { return rec.Execution.NodeName == nodeName }), nil
}

func (r *Runtime) Of(ctx context.Context, taskUUID string) ([]task.Execution, error) {
	return r.matching(func(rec record) bool { return rec.Execution.TaskUUID == taskUUID }), nil
}

func (r *Runtime) BySlug(ctx context.Context, slug string) ([]task.Execution, error) {
	return r.matching(func(rec record) bool { return rec.Execution.Slug == slug }), nil
}

func (r *Runtime) matching(matches func(record) bool) []task.Execution {
	var executions []task.Execution

	for _, rec := range r.store.all() {
		if matches(rec) {
			executions = append(executions, rec.execution())
		}
	}

	return executions
}

func (r *Runtime) EnsureImage(ctx context.Context, reference string) error {
	_, err := r.images.Ensure(ctx, reference)

	return err
}

// Create makes a machine for a task: its record, and its scratch disk. It
// boots nothing: Start does.
func (r *Runtime) Create(ctx context.Context, execution *task.Execution) (string, error) {
	ctx, span := r.tracer.Start(ctx, "firecracker.task.create",
		oteltrace.WithAttributes(attribute.String("image", execution.Image), attribute.String("name", execution.Name)),
	)
	defer span.End()

	for _, existing := range r.store.all() {
		if existing.Execution.Name == execution.Name {
			return "", trace.RecordError(span, fmt.Errorf("a machine called %s is already there", execution.Name))
		}
	}

	built, err := r.images.Ensure(ctx, execution.Image)
	if err != nil {
		return "", trace.RecordError(span, err)
	}

	process, err := resolveProcess(built.Config, execution)
	if err != nil {
		return "", trace.RecordError(span, err)
	}

	id, err := newID()
	if err != nil {
		return "", trace.RecordError(span, err)
	}

	vcpus, quota, memoryMiB := resources(execution.ResourceLimits)

	rec := record{
		Execution: *execution,
		Process:   process,
		Hostname:  hostnameOf(execution, id),
		Image:     built.Root,
		VCPUs:     vcpus,
		CPUQuota:  quota,
		MemoryMiB: memoryMiB,
		Status:    task.StatusCreated,
		CreatedAt: time.Now().UTC(),
	}

	rec.Execution.ID = id

	if !execution.ReadOnly {
		rec.Scratch = filepath.Join(r.store.machineDir(id), scratchName)

		if err := r.makeScratch(ctx, rec.Scratch, scratchBytes(execution.ResourceLimits)); err != nil {
			return "", trace.RecordError(span, errors.Join(err, r.store.remove(id)))
		}
	}

	if err := r.store.put(rec); err != nil {
		return "", trace.RecordError(span, errors.Join(err, r.store.remove(id)))
	}

	r.logger.Info("machine created", "machine", id, "name", execution.Name, "image", execution.Image)

	return id, nil
}

// makeScratch makes the disk a writable task keeps its changes on: empty,
// sparse, and as large as it may write.
func (r *Runtime) makeScratch(ctx context.Context, path string, size int64) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(path, nil, 0o644); err != nil {
		return err
	}

	if err := os.Truncate(path, size); err != nil {
		return err
	}

	output, err := exec.CommandContext(ctx, "mke2fs", "-q", "-F", "-t", "ext4", "-L", "scratch", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to make a scratch disk: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return r.own(path)
}

// own makes a file whoever machines run as, when this process may give it
// away: one that runs as them already makes its files theirs.
func (r *Runtime) own(path string) error {
	if os.Geteuid() != 0 {
		return nil
	}

	return os.Chown(path, r.config.UID, r.config.GID)
}

func (r *Runtime) Start(ctx context.Context, id string) error {
	ctx, span := r.tracer.Start(ctx, "firecracker.task.start",
		oteltrace.WithAttributes(attribute.String("task.id", id)),
	)
	defer span.End()

	unlock := r.locks.lock(id)
	defer unlock()

	return trace.RecordError(span, r.start(ctx, id))
}

// start boots a machine and runs its task. The lock is held.
func (r *Runtime) start(ctx context.Context, id string) error {
	rec, found := r.store.get(id)
	if !found {
		return errNoMachine
	}

	// starting a machine that runs is starting nothing, as for a container.
	if r.keeper(id) != nil {
		return nil
	}

	interfaces, err := r.leaseInterfaces(ctx, rec)
	if err != nil {
		return errors.Join(err, r.networks.release(id))
	}

	rec.Interfaces = interfaces

	launched, err := r.launcher.Launch(ctx, r.specOf(rec))
	if err != nil {
		return errors.Join(err, r.networks.release(id))
	}

	fail := func(err error) error {
		letGo := context.WithoutCancel(ctx)

		return errors.Join(err, r.launcher.Terminate(letGo, id), r.networks.release(id))
	}

	if err := r.boot(ctx, launched, rec); err != nil {
		return fail(err)
	}

	client := guest.NewClient(filepath.Join(launched.Root, vsockName))

	ready, cancel := context.WithTimeout(ctx, bootTimeout)
	defer cancel()

	if err := client.Ready(ready); err != nil {
		return fail(fmt.Errorf("the machine did not come up: %w", err))
	}

	if err := client.Configure(ctx, r.guestConfig(rec)); err != nil {
		return fail(err)
	}

	out, err := openOutput(filepath.Join(r.store.machineDir(id), outputName), outputLimit)
	if err != nil {
		return fail(err)
	}

	status, err := client.Start(ctx, rec.Process)
	if err != nil {
		out.close()

		// a task that could not be started at all has ended, the way a
		// container whose command is not there has.
		_, _ = r.store.update(id, func(rec *record) {
			rec.Status = task.StatusExited
			rec.ExitCode = 127
			rec.FinishedAt = time.Now().UTC()
		})

		return fail(err)
	}

	updated, err := r.store.update(id, func(rec *record) {
		rec.Status = task.StatusRunning
		rec.Interfaces = interfaces
		rec.StartedAt = status.StartedAt
		rec.FinishedAt = time.Time{}
		rec.ExitCode = 0
		rec.Generation = status.Generation
		rec.LogBase = out.lastSeq()
		rec.Stopped = false
	})
	if err != nil {
		out.close()

		return fail(err)
	}

	r.attach(updated, client, out)
	r.refreshHosts(ctx, updated.Interfaces)

	r.logger.Info("machine started", "machine", id, "name", rec.Execution.Name)

	return nil
}

// leaseInterfaces gives a machine an address on every network its task
// joins.
func (r *Runtime) leaseInterfaces(ctx context.Context, rec record) ([]iface, error) {
	var interfaces []iface

	for _, attachment := range rec.Execution.Networks {
		if attachment.Name == network.NoNetworkName {
			continue
		}

		leased, err := r.networks.lease(ctx, attachment, rec.Execution.ID)
		if err != nil {
			return nil, err
		}

		interfaces = append(interfaces, leased)
	}

	return interfaces, nil
}

// specOf is the machine the launcher is asked for.
func (r *Runtime) specOf(rec record) machine.Spec {
	taps := make([]machine.Tap, len(rec.Interfaces))
	for i, leased := range rec.Interfaces {
		taps[i] = machine.Tap{Network: leased.Network}
	}

	drives := []string{rec.Image}
	if len(rec.Scratch) > 0 {
		drives = append(drives, rec.Scratch)
	}

	return machine.Spec{
		ID:        rec.Execution.ID,
		Owner:     r.config.Owner,
		VCPUs:     rec.VCPUs,
		CPUQuota:  rec.CPUQuota,
		MemoryMiB: rec.MemoryMiB,
		Taps:      taps,
		Files:     machine.Files{Kernel: r.config.Kernel, Initrd: r.config.Initrd, Drives: drives},
	}
}

// guestConfig is what a machine is told it is.
func (r *Runtime) guestConfig(rec record) guest.Config {
	config := guest.Config{
		Now:      time.Now().UTC(),
		Hostname: rec.Hostname,
		Root:     guest.Root{Image: imageDevice},
		Hosts:    r.hostsFor(rec),
	}

	if len(rec.Scratch) > 0 {
		config.Root.Scratch = scratchDevice
	}

	for _, leased := range rec.Interfaces {
		config.Interfaces = append(config.Interfaces, guest.Interface{
			MAC:     leased.MAC,
			Address: leased.Address,
			Gateway: leased.Gateway,
		})

		// a machine that cannot reach the internet is given nowhere to ask
		// it for names.
		if len(leased.Gateway) > 0 {
			config.Nameservers = r.config.Nameservers
		}
	}

	return config
}

// hostsFor is the names a machine's neighbours answer to, on every network it
// shares with them under a name of their own: the services of its stack.
func (r *Runtime) hostsFor(rec record) []guest.Host {
	var hosts []guest.Host

	for _, other := range r.store.all() {
		if other.Execution.ID == rec.Execution.ID || !other.running() {
			continue
		}

		for _, theirs := range other.Interfaces {
			if len(theirs.Aliases) == 0 || !sharesNetwork(rec.Interfaces, theirs.Network) {
				continue
			}

			address, _, _ := cutPrefix(theirs.Address)
			hosts = append(hosts, guest.Host{Address: address, Names: append(slices.Clone(theirs.Aliases), other.Hostname)})
		}
	}

	return hosts
}

func sharesNetwork(interfaces []iface, name string) bool {
	return slices.ContainsFunc(interfaces, func(i iface) bool { return i.Network == name })
}

// refreshHosts tells every machine on the networks given who its neighbours
// now are, which is what changes when one of them comes or goes.
func (r *Runtime) refreshHosts(ctx context.Context, interfaces []iface) {
	for _, other := range r.store.all() {
		if !other.running() {
			continue
		}

		shares := slices.ContainsFunc(interfaces, func(i iface) bool { return sharesNetwork(other.Interfaces, i.Network) })
		if !shares {
			continue
		}

		k := r.keeper(other.Execution.ID)
		if k == nil {
			continue
		}

		if err := k.client.SetHosts(ctx, r.hostsFor(other)); err != nil {
			r.logger.Warn("a machine could not be told who its neighbours are", "machine", other.Execution.ID, "error", err)
		}
	}
}

func (r *Runtime) Stop(ctx context.Context, id string) error {
	ctx, span := r.tracer.Start(ctx, "firecracker.task.stop",
		oteltrace.WithAttributes(attribute.String("task.id", id)),
	)
	defer span.End()

	unlock := r.locks.lock(id)
	defer unlock()

	return trace.RecordError(span, r.halt(ctx, id, func(k *keeper) error {
		_, err := k.client.Stop(ctx, stopTimeout)

		return err
	}))
}

// Restart stops a machine's task and starts it again, in a machine booted
// anew from the same disks: what the task wrote to its root survives, as it
// does a container's restart.
func (r *Runtime) Restart(ctx context.Context, id string) error {
	ctx, span := r.tracer.Start(ctx, "firecracker.task.restart",
		oteltrace.WithAttributes(attribute.String("task.id", id)),
	)
	defer span.End()

	unlock := r.locks.lock(id)
	defer unlock()

	err := r.halt(ctx, id, func(k *keeper) error {
		_, err := k.client.Stop(ctx, stopTimeout)

		return err
	})
	if err != nil {
		return trace.RecordError(span, err)
	}

	return trace.RecordError(span, r.start(ctx, id))
}

// Kill ends a machine's task at once, without the grace Stop gives it.
func (r *Runtime) Kill(ctx context.Context, id string) error {
	ctx, span := r.tracer.Start(ctx, "firecracker.task.kill",
		oteltrace.WithAttributes(attribute.String("task.id", id)),
	)
	defer span.End()

	unlock := r.locks.lock(id)
	defer unlock()

	return trace.RecordError(span, r.halt(ctx, id, func(k *keeper) error {
		return k.client.Signal(ctx, 9)
	}))
}

// halt ends a machine's task the way end does, on purpose, and waits for the
// machine to be let go. A task that ended on purpose is not started again by
// its restart policy. A machine that does not answer is ended from outside.
// The lock is held.
func (r *Runtime) halt(ctx context.Context, id string, end func(*keeper) error) error {
	if _, found := r.store.get(id); !found {
		return errNoMachine
	}

	k := r.keeper(id)
	if k == nil {
		return nil
	}

	if _, err := r.store.update(id, func(rec *record) { rec.Stopped = true }); err != nil {
		return err
	}

	if err := end(k); err != nil && !errors.Is(err, guest.ErrNotRunning) {
		r.logger.Warn("the machine did not answer, and is ended from outside", "machine", id, "error", err)

		k.abandon()
		r.finalize(k, guest.Status{ExitCode: lostExitCode, FinishedAt: time.Now().UTC()}, false)

		return nil
	}

	select {
	case <-k.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Delete takes a machine away, and everything kept for it. A machine that
// runs is ended first.
func (r *Runtime) Delete(ctx context.Context, id string) error {
	ctx, span := r.tracer.Start(ctx, "firecracker.task.delete",
		oteltrace.WithAttributes(attribute.String("task.id", id)),
	)
	defer span.End()

	unlock := r.locks.lock(id)
	defer unlock()

	rec, found := r.store.get(id)
	if !found {
		return trace.RecordError(span, errNoMachine)
	}

	if k := r.keeper(id); k != nil {
		k.abandon()
		r.detach(id)
		k.close()
	}

	err := errors.Join(
		r.launcher.Terminate(ctx, id),
		r.networks.release(id),
		r.store.remove(id),
	)
	if err != nil {
		return trace.RecordError(span, err)
	}

	r.refreshHosts(ctx, rec.Interfaces)
	r.locks.forget(id)

	r.logger.Info("machine deleted", "machine", id)

	return nil
}

func (r *Runtime) Inspect(ctx context.Context, id string) (task.Execution, error) {
	rec, found := r.store.get(id)
	if !found {
		return task.Execution{}, errNoMachine
	}

	return rec.execution(), nil
}

func (r *Runtime) Stats(ctx context.Context, id string) (task.Stats, error) {
	ctx, span := r.tracer.Start(ctx, "firecracker.task.stats",
		oteltrace.WithAttributes(attribute.String("task.id", id)),
	)
	defer span.End()

	if _, found := r.store.get(id); !found {
		return task.Stats{}, trace.RecordError(span, errNoMachine)
	}

	// a machine that does not run uses nothing, as a stopped container does.
	k := r.keeper(id)
	if k == nil {
		return task.Stats{}, nil
	}

	used, err := k.client.Stats(ctx)
	if err != nil {
		return task.Stats{}, trace.RecordError(span, err)
	}

	stats := task.Stats{
		PIDs:          used.PIDs,
		CPUPercent:    used.CPUPercent,
		MemoryUsage:   used.MemoryUsage,
		MemoryLimit:   used.MemoryLimit,
		NetworkInput:  used.NetworkInput,
		NetworkOutput: used.NetworkOutput,
		BlockInput:    used.BlockInput,
		BlockOutput:   used.BlockOutput,
	}

	if used.MemoryLimit > 0 {
		stats.MemoryPercent = float64(used.MemoryUsage) / float64(used.MemoryLimit) * 100
	}

	return stats, nil
}

func (r *Runtime) Logs(ctx context.Context, id string, writer io.Writer) error {
	if _, found := r.store.get(id); !found {
		return errNoMachine
	}

	f := &follower{path: filepath.Join(r.store.machineDir(id), outputName)}

	return f.next(func(line guest.LogLine) error {
		_, err := io.WriteString(writer, line.Content+"\n")

		return err
	})
}

// StreamLogs follows a machine's output from since onward, until the task
// ends, emit refuses a line, or ctx is done.
func (r *Runtime) StreamLogs(ctx context.Context, id string, since time.Time, emit func(task.LogLine) error) error {
	if _, found := r.store.get(id); !found {
		return errNoMachine
	}

	f := &follower{path: filepath.Join(r.store.machineDir(id), outputName)}

	forward := func(line guest.LogLine) error {
		if line.At.Before(since) {
			return nil
		}

		stream := task.StreamStdout
		if line.Stream == guest.StreamStderr {
			stream = task.StreamStderr
		}

		return emit(task.LogLine{Stream: stream, Content: line.Content, At: line.At})
	}

	for {
		// what is waited on is taken before reading, so a line kept in
		// between is not missed.
		var changed <-chan struct{}
		if k := r.keeper(id); k != nil {
			changed = k.out.wait()
		}

		if err := f.next(forward); err != nil {
			return err
		}

		rec, found := r.store.get(id)
		if !found || (!rec.running() && r.keeper(id) == nil) {
			return f.next(forward)
		}

		select {
		case <-changed:
		case <-time.After(pollInterval):
		case <-ctx.Done():
			return nil
		}
	}
}

func (r *Runtime) Exec(ctx context.Context, id string, options task.ExecOptions) (task.ExecSession, error) {
	ctx, span := r.tracer.Start(ctx, "firecracker.task.exec",
		oteltrace.WithAttributes(attribute.String("task.id", id)),
	)
	defer span.End()

	k := r.keeper(id)
	if k == nil {
		return nil, trace.RecordError(span, fmt.Errorf("machine %s: %w", id, guest.ErrNotRunning))
	}

	stream, err := k.client.Exec(ctx, guest.Exec{
		Process: guest.Process{Args: options.Command, Env: options.Env, WorkingDir: options.WorkDir},
		TTY:     options.TTY,
	})
	if err != nil {
		return nil, trace.RecordError(span, err)
	}

	return &execSession{stream: stream, client: k.client}, nil
}

// Dial connects to one of the ports a machine's task exposes, through its
// agent: nothing on the host reaches into a machine's network.
func (r *Runtime) Dial(ctx context.Context, id string, p port.Port) (net.Conn, error) {
	ctx, span := r.tracer.Start(ctx, "firecracker.task.dial",
		oteltrace.WithAttributes(attribute.String("task.id", id), attribute.Int("task.port", int(p))),
	)
	defer span.End()

	rec, found := r.store.get(id)
	if !found {
		return nil, trace.RecordError(span, errNoMachine)
	}

	if !slices.Contains(rec.execution().Endpoints, p) {
		return nil, trace.RecordError(span, fmt.Errorf("port %d of machine %s cannot be reached", p, id))
	}

	k := r.keeper(id)
	if k == nil {
		return nil, trace.RecordError(span, fmt.Errorf("machine %s: %w", id, guest.ErrNotRunning))
	}

	conn, err := k.client.Dial(ctx, uint16(p))
	if err != nil {
		return nil, trace.RecordError(span, err)
	}

	return conn, nil
}

func (r *Runtime) keeper(id string) *keeper {
	r.lock.Lock()
	defer r.lock.Unlock()

	return r.keepers[id]
}

func (r *Runtime) detach(id string) {
	r.lock.Lock()
	defer r.lock.Unlock()

	delete(r.keepers, id)
}

// hostnameOf is what a machine calls itself: the name its task's ports are
// served under, or failing that its own.
func hostnameOf(execution *task.Execution, id string) string {
	if len(execution.Slug) > 0 {
		return execution.Slug
	}

	return id[:12]
}

// newID names a machine: sixteen hex digits, which is what the launcher takes.
func newID() (string, error) {
	id := make([]byte, 8)
	if _, err := rand.Read(id); err != nil {
		return "", err
	}

	return hex.EncodeToString(id), nil
}

// keyedLock serialises what is done to one machine, and nothing else.
type keyedLock struct {
	guard sync.Mutex
	held  map[string]*sync.Mutex
}

func (k *keyedLock) lock(key string) func() {
	k.guard.Lock()

	if k.held == nil {
		k.held = make(map[string]*sync.Mutex)
	}

	held, found := k.held[key]
	if !found {
		held = &sync.Mutex{}
		k.held[key] = held
	}

	k.guard.Unlock()

	held.Lock()

	return held.Unlock
}

// forget lets go of a machine that is gone.
func (k *keyedLock) forget(key string) {
	k.guard.Lock()
	defer k.guard.Unlock()

	delete(k.held, key)
}
