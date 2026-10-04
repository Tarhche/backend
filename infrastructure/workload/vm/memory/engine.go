// Package memory is an engine that runs nothing: its instances live in a map,
// and it does what it is asked the way an engine would.
//
// It is for tests. What uses vm.Engine is tested against something that
// behaves like one, rather than against a mock told what to answer: an
// instance that was never created is not there, one that is stopped cannot be
// exec'd into, a restore of an archive another engine wrote is refused, and a
// node asked for more than its budget has no capacity. What would run inside
// an instance is the test's to say: Exec runs a function the test gives it, so
// `docker system dial-stdio` can be a connection to an httptest Docker API, or
// `docker compose` a function that reads the YAML and prints what compose
// would.
package memory

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Name and Version are this engine's, as the archives it writes say: a restore
// takes only an archive written by the same engine.
const (
	Name    = "memory"
	Version = "1"
)

// firstHostPort is where the host ports given to published guest ports start,
// as a vmhost's range does.
const firstHostPort = 20000

// ExecFunc is what runs inside an instance when a command is exec'd into it.
//
// It reads the command's input from stdin and writes its output to stdout and
// stderr, which are the same writer under a TTY. What it returns is the
// command's exit code. Its context ends when the session is closed or the
// instance stops, which is when a command that is still running has to give
// up.
type ExecFunc func(ctx context.Context, id string, options vm.ExecOptions, stdin io.Reader, stdout io.Writer, stderr io.Writer) int

// MainFunc is an instance's main process: what runs once an instance whose
// spec has a Command boots. It writes its log with log, under the main source,
// and what it returns is its exit code, which the instance keeps once it has
// exited. Its context ends when the instance is stopped or deleted.
type MainFunc func(ctx context.Context, spec vm.Spec, log func(line string)) int

// commandNotFound is what a shell answers for a command it does not have, and
// so what an exec answers when the test has not said what runs inside.
const commandNotFound = 127

// Option configures an Engine.
type Option func(*Engine)

// WithCapacity is the budget the engine offers: whole vCPUs, and bytes of
// memory and disk. A zero is no limit. CPUs are never refused, since a node's
// CPUs are shared rather than given away; memory and disk are.
func WithCapacity(cpus uint, memory uint64, disk uint64) Option {
	return func(e *Engine) {
		e.budget.CPUs = cpus
		e.budget.Memory = memory
		e.budget.Disk = disk
	}
}

// WithExec is what runs inside an instance when a command is exec'd into it.
func WithExec(exec ExecFunc) Option {
	return func(e *Engine) {
		e.exec = exec
	}
}

// WithMain is what an instance's main process does. Without it, the main
// process runs until the test says it exited, with Exit.
func WithMain(main MainFunc) Option {
	return func(e *Engine) {
		e.main = main
	}
}

// WithHost is the host the endpoints of published ports name, as a vmhost's
// advertise host does.
func WithHost(host string) Option {
	return func(e *Engine) {
		e.host = host
	}
}

// WithClock is what the engine reads the time from.
func WithClock(now func() time.Time) Option {
	return func(e *Engine) {
		e.now = now
	}
}

// Engine is a vm.Engine whose instances live in memory.
type Engine struct {
	lock sync.Mutex

	budget    vm.Info
	host      string
	nextPort  int
	instances map[string]*instance

	exec ExecFunc
	main MainFunc
	now  func() time.Time
}

var _ vm.Engine = &Engine{}

// instance is one of the engine's instances, with what it was given and what
// it has become.
type instance struct {
	spec vm.Spec

	state     vm.InstanceState
	exitCode  int
	reason    string
	startedAt time.Time

	// hostPorts is the host port each guest port was given. It is kept for as
	// long as the instance is, so a restart or a restore keeps its ports.
	hostPorts map[port.Port]int

	logs  []vm.LogLine
	stats vm.Stats
	disk  []byte

	// boot counts the times the instance has booted, so a main process of an
	// earlier boot ending is not taken for the current one's.
	boot     int
	stopMain context.CancelFunc

	sessions map[*Session]struct{}
}

// New is an engine holding nothing, with no limit to what it offers unless an
// option says otherwise.
func New(options ...Option) *Engine {
	e := &Engine{
		budget:    vm.Info{Engine: Name, Version: Version},
		host:      "vmhost",
		nextPort:  firstHostPort,
		instances: make(map[string]*instance),
		now:       time.Now,
	}

	for _, option := range options {
		option(e)
	}

	return e
}

// archiveEngine is what an archive this engine writes says wrote it.
func archiveEngine() string {
	return Name + "/" + Version
}

func (e *Engine) Info(ctx context.Context) (vm.Info, error) {
	e.lock.Lock()
	defer e.lock.Unlock()

	info := e.budget
	for _, i := range e.instances {
		info.Allocated.CPUs += i.spec.Resources.CPUs
		info.Allocated.Memory += i.spec.Resources.Memory
		info.Allocated.Disk += i.spec.Resources.Disk
	}

	return info, nil
}

func (e *Engine) List(ctx context.Context) ([]vm.Instance, error) {
	e.lock.Lock()
	defer e.lock.Unlock()

	ids := slices.Sorted(maps.Keys(e.instances))

	instances := make([]vm.Instance, len(ids))
	for n, id := range ids {
		instances[n] = e.view(id, e.instances[id])
	}

	return instances, nil
}

func (e *Engine) Inspect(ctx context.Context, id string) (vm.Instance, error) {
	e.lock.Lock()
	defer e.lock.Unlock()

	i, err := e.get(id)
	if err != nil {
		return vm.Instance{}, err
	}

	return e.view(id, i), nil
}

func (e *Engine) Create(ctx context.Context, spec vm.Spec) (vm.Instance, error) {
	if len(spec.ID) == 0 {
		return vm.Instance{}, errors.New("an instance is created under an id")
	}

	e.lock.Lock()
	defer e.lock.Unlock()

	if _, exists := e.instances[spec.ID]; exists {
		return vm.Instance{}, fmt.Errorf("%w: an instance %q", domain.ErrAlreadyExists, spec.ID)
	}

	if err := e.fits(spec.Resources, ""); err != nil {
		return vm.Instance{}, err
	}

	i := &instance{
		spec:      cloneSpec(spec),
		hostPorts: make(map[port.Port]int),
		sessions:  make(map[*Session]struct{}),
	}

	e.publish(i)
	e.instances[spec.ID] = i
	e.boot(spec.ID, i)

	return e.view(spec.ID, i), nil
}

func (e *Engine) Start(ctx context.Context, id string) error {
	e.lock.Lock()
	defer e.lock.Unlock()

	i, err := e.get(id)
	if err != nil {
		return err
	}

	if i.state != vm.InstanceRunning {
		e.boot(id, i)
	}

	return nil
}

func (e *Engine) Stop(ctx context.Context, id string) error {
	e.lock.Lock()
	defer e.lock.Unlock()

	i, err := e.get(id)
	if err != nil {
		return err
	}

	// one whose main process exited keeps saying so, and its exit code; one
	// that failed keeps its reason.
	if i.state == vm.InstanceRunning || i.state == vm.InstanceCreated {
		e.halt(i, vm.InstanceStopped)
	}

	return nil
}

func (e *Engine) Restart(ctx context.Context, id string) error {
	e.lock.Lock()
	defer e.lock.Unlock()

	i, err := e.get(id)
	if err != nil {
		return err
	}

	e.halt(i, vm.InstanceStopped)
	e.boot(id, i)

	return nil
}

func (e *Engine) Delete(ctx context.Context, id string) error {
	e.lock.Lock()
	defer e.lock.Unlock()

	i, ok := e.instances[id]
	if !ok {
		return nil
	}

	e.halt(i, vm.InstanceStopped)
	delete(e.instances, id)

	return nil
}

// Reconfigure applies a spec's ports, network and resources, and restarts a
// running instance for them to take, which is what an engine that cannot
// change a running VM does.
func (e *Engine) Reconfigure(ctx context.Context, spec vm.Spec) (vm.Instance, error) {
	e.lock.Lock()
	defer e.lock.Unlock()

	i, err := e.get(spec.ID)
	if err != nil {
		return vm.Instance{}, err
	}

	if err := e.fits(spec.Resources, spec.ID); err != nil {
		return vm.Instance{}, err
	}

	i.spec.Ports = slices.Clone(spec.Ports)
	i.spec.Network = spec.Network
	i.spec.Resources = spec.Resources
	e.publish(i)

	if i.state == vm.InstanceRunning {
		e.halt(i, vm.InstanceStopped)
		e.boot(spec.ID, i)
	}

	return e.view(spec.ID, i), nil
}

// Stats is what a running instance is using: what the test set with SetStats,
// with its memory limit and disk total being what it was given. CPUPercent
// keeps the engine's meaning: 0 to 100 of the instance's vCPUs together.
func (e *Engine) Stats(ctx context.Context, id string) (vm.Stats, error) {
	e.lock.Lock()
	defer e.lock.Unlock()

	i, err := e.get(id)
	if err != nil {
		return vm.Stats{}, err
	}

	if i.state != vm.InstanceRunning {
		return vm.Stats{}, fmt.Errorf("%w: %q is %s", vm.ErrNotRunning, id, i.state)
	}

	stats := i.stats
	stats.MemoryLimit = cmp.Or(stats.MemoryLimit, i.spec.Resources.Memory)
	stats.DiskTotal = cmp.Or(stats.DiskTotal, i.spec.Resources.Disk)
	stats.SampledAt = e.now()

	return stats, nil
}

func (e *Engine) Logs(ctx context.Context, id string, options vm.LogOptions) ([]vm.LogLine, error) {
	e.lock.Lock()
	defer e.lock.Unlock()

	i, err := e.get(id)
	if err != nil {
		return nil, err
	}

	lines := make([]vm.LogLine, 0, len(i.logs))
	for _, line := range i.logs {
		if !options.Since.IsZero() && line.At.Before(options.Since) {
			continue
		}

		lines = append(lines, line)
	}

	if options.Tail > 0 && uint(len(lines)) > options.Tail {
		lines = lines[uint(len(lines))-options.Tail:]
	}

	return lines, nil
}

// Exec runs the engine's ExecFunc as a command inside a running instance.
func (e *Engine) Exec(ctx context.Context, id string, options vm.ExecOptions) (vm.ExecSession, error) {
	e.lock.Lock()
	defer e.lock.Unlock()

	i, err := e.get(id)
	if err != nil {
		return nil, err
	}

	if i.state != vm.InstanceRunning {
		return nil, fmt.Errorf("%w: %q is %s", vm.ErrNotRunning, id, i.state)
	}

	exec := e.exec
	if exec == nil {
		exec = func(context.Context, string, vm.ExecOptions, io.Reader, io.Writer, io.Writer) int {
			return commandNotFound
		}
	}

	session := newSession(options)
	i.sessions[session] = struct{}{}

	session.start(func(ctx context.Context, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
		return exec(ctx, id, options, stdin, stdout, stderr)
	}, func() {
		e.lock.Lock()
		defer e.lock.Unlock()

		delete(i.sessions, session)
	})

	return session, nil
}

// archive is what Snapshot writes and Restore reads: everything an instance is,
// disk included, so the archive is all a restore needs.
type archive struct {
	Engine string  `json:"engine"`
	Spec   vm.Spec `json:"spec"`
	Disk   []byte  `json:"disk"`
}

func (e *Engine) Snapshot(ctx context.Context, id string, writer io.Writer) (vm.Archive, error) {
	e.lock.Lock()
	i, err := e.get(id)
	if err != nil {
		e.lock.Unlock()

		return vm.Archive{}, err
	}

	written := archive{Engine: archiveEngine(), Spec: cloneSpec(i.spec), Disk: slices.Clone(i.disk)}
	e.lock.Unlock()

	counted := &countingWriter{writer: writer}
	if err := json.NewEncoder(counted).Encode(written); err != nil {
		return vm.Archive{}, err
	}

	return vm.Archive{
		Engine: written.Engine,
		Kind:   written.Spec.Kind,
		Image:  written.Spec.Image,
		Disk:   written.Spec.Resources.Disk,
		Size:   counted.written,
	}, nil
}

// Restore replaces the instance spec.ID names with what the archive holds, or
// creates it. A replaced instance keeps its host ports, as a restored VM keeps
// its ports.
func (e *Engine) Restore(ctx context.Context, spec vm.Spec, reader io.Reader) (vm.Instance, error) {
	if len(spec.ID) == 0 {
		return vm.Instance{}, errors.New("an instance is restored under an id")
	}

	var read archive
	if err := json.NewDecoder(reader).Decode(&read); err != nil {
		return vm.Instance{}, fmt.Errorf("the archive cannot be read: %w", err)
	}

	if read.Engine != archiveEngine() {
		return vm.Instance{}, fmt.Errorf("%w: written by %q", vm.ErrEngineMismatch, read.Engine)
	}

	e.lock.Lock()
	defer e.lock.Unlock()

	if err := e.fits(spec.Resources, spec.ID); err != nil {
		return vm.Instance{}, err
	}

	i, exists := e.instances[spec.ID]
	if exists {
		e.halt(i, vm.InstanceStopped)
		i.logs = nil
		i.stats = vm.Stats{}
	} else {
		i = &instance{hostPorts: make(map[port.Port]int), sessions: make(map[*Session]struct{})}
		e.instances[spec.ID] = i
	}

	i.spec = cloneSpec(spec)
	i.disk = read.Disk
	e.publish(i)
	e.boot(spec.ID, i)

	return e.view(spec.ID, i), nil
}

// Exit ends an instance's main process with exitCode, as the process ending
// on its own would.
func (e *Engine) Exit(id string, exitCode int) error {
	e.lock.Lock()
	defer e.lock.Unlock()

	i, err := e.get(id)
	if err != nil {
		return err
	}

	e.halt(i, vm.InstanceExited)
	i.exitCode = exitCode

	return nil
}

// Fail makes an instance one that failed, for reason.
func (e *Engine) Fail(id string, reason string) error {
	e.lock.Lock()
	defer e.lock.Unlock()

	i, err := e.get(id)
	if err != nil {
		return err
	}

	e.halt(i, vm.InstanceFailed)
	i.reason = reason

	return nil
}

// Log writes a line to an instance's log, from source.
func (e *Engine) Log(id string, source string, line string) error {
	e.lock.Lock()
	defer e.lock.Unlock()

	i, err := e.get(id)
	if err != nil {
		return err
	}

	i.logs = append(i.logs, vm.LogLine{At: e.now(), Source: source, Line: line})

	return nil
}

// SetStats is what an instance says it is using from now on. CPUPercent is 0
// to 100 of all of the instance's vCPUs, as the engine reports it.
func (e *Engine) SetStats(id string, stats vm.Stats) error {
	e.lock.Lock()
	defer e.lock.Unlock()

	i, err := e.get(id)
	if err != nil {
		return err
	}

	i.stats = stats

	return nil
}

// SetDisk is what is on an instance's disk, which a snapshot archives and a
// restore brings back.
func (e *Engine) SetDisk(id string, disk []byte) error {
	e.lock.Lock()
	defer e.lock.Unlock()

	i, err := e.get(id)
	if err != nil {
		return err
	}

	i.disk = slices.Clone(disk)

	return nil
}

// Disk is what is on an instance's disk.
func (e *Engine) Disk(id string) ([]byte, error) {
	e.lock.Lock()
	defer e.lock.Unlock()

	i, err := e.get(id)
	if err != nil {
		return nil, err
	}

	return slices.Clone(i.disk), nil
}

// Spec is what an instance was last given: what it was created, reconfigured
// or restored with.
func (e *Engine) Spec(id string) (vm.Spec, error) {
	e.lock.Lock()
	defer e.lock.Unlock()

	i, err := e.get(id)
	if err != nil {
		return vm.Spec{}, err
	}

	return cloneSpec(i.spec), nil
}

// Sessions is how many exec sessions into an instance are still open.
func (e *Engine) Sessions(id string) int {
	e.lock.Lock()
	defer e.lock.Unlock()

	i, ok := e.instances[id]
	if !ok {
		return 0
	}

	return len(i.sessions)
}

// get is the instance named id. It is called with the lock held.
func (e *Engine) get(id string) (*instance, error) {
	i, ok := e.instances[id]
	if !ok {
		return nil, fmt.Errorf("%w: no instance %q", domain.ErrNotExists, id)
	}

	return i, nil
}

// fits refuses what would take this engine's memory or disk past its budget,
// counting everything it holds but the instance named except, which is the one
// being changed. It is called with the lock held.
func (e *Engine) fits(asked vm.Resources, except string) error {
	var memory, disk uint64
	for id, i := range e.instances {
		if id == except {
			continue
		}

		memory += i.spec.Resources.Memory
		disk += i.spec.Resources.Disk
	}

	if e.budget.Memory > 0 && memory+asked.Memory > e.budget.Memory {
		return fmt.Errorf("%w: %d bytes of memory asked, %d of %d given", vm.ErrNoCapacity, asked.Memory, memory, e.budget.Memory)
	}

	if e.budget.Disk > 0 && disk+asked.Disk > e.budget.Disk {
		return fmt.Errorf("%w: %d bytes of disk asked, %d of %d given", vm.ErrNoCapacity, asked.Disk, disk, e.budget.Disk)
	}

	return nil
}

// publish gives every guest port of an instance a host port, keeping the ones
// it already had and letting go of the ones it no longer has. It is called
// with the lock held.
func (e *Engine) publish(i *instance) {
	for guest := range i.hostPorts {
		if !slices.Contains(i.spec.Ports, guest) {
			delete(i.hostPorts, guest)
		}
	}

	for _, guest := range i.spec.Ports {
		if _, given := i.hostPorts[guest]; given {
			continue
		}

		i.hostPorts[guest] = e.nextPort
		e.nextPort++
	}
}

// boot brings an instance up, and its main process with it when it has one. It
// is called with the lock held.
func (e *Engine) boot(id string, i *instance) {
	i.boot++
	i.state = vm.InstanceRunning
	i.exitCode = 0
	i.reason = ""
	i.startedAt = e.now()

	if len(i.spec.Command) == 0 || e.main == nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	i.stopMain = cancel

	boot := i.boot
	spec := cloneSpec(i.spec)

	go func() {
		exitCode := e.main(ctx, spec, func(line string) {
			_ = e.Log(id, vm.LogSourceMain, line)
		})

		e.lock.Lock()
		defer e.lock.Unlock()

		// stopped, restarted or deleted meanwhile: what this boot's process
		// returned is not what the instance is doing now.
		if current, ok := e.instances[id]; !ok || current != i || i.boot != boot || i.state != vm.InstanceRunning {
			return
		}

		i.state = vm.InstanceExited
		i.exitCode = exitCode
		i.stopMain = nil
	}()
}

// halt takes an instance down: its main process ends and every session into
// it is closed, as a VM going down takes what runs in it along. It is called
// with the lock held.
func (e *Engine) halt(i *instance, state vm.InstanceState) {
	if i.stopMain != nil {
		i.stopMain()
		i.stopMain = nil
	}

	for session := range i.sessions {
		_ = session.Close()
	}

	i.state = state
}

// view is an instance as the engine reports it. It is called with the lock
// held.
func (e *Engine) view(id string, i *instance) vm.Instance {
	instance := vm.Instance{
		ID:        id,
		State:     i.state,
		Labels:    maps.Clone(i.spec.Labels),
		Reason:    i.reason,
		StartedAt: i.startedAt,
	}

	if i.state == vm.InstanceExited {
		instance.ExitCode = i.exitCode
	}

	// nothing reaches an instance whose ingress is denied, so nothing of it
	// is published.
	if i.spec.Network.Ingress != vm.AccessAllow {
		return instance
	}

	guests := slices.Sorted(maps.Keys(i.hostPorts))
	for _, guest := range guests {
		instance.Endpoints = append(instance.Endpoints, vm.Endpoint{
			Port:    guest,
			Address: net.JoinHostPort(e.host, strconv.Itoa(i.hostPorts[guest])),
		})
	}

	return instance
}

func cloneSpec(spec vm.Spec) vm.Spec {
	spec.Ports = slices.Clone(spec.Ports)
	spec.Labels = maps.Clone(spec.Labels)
	spec.Command = slices.Clone(spec.Command)
	spec.Env = slices.Clone(spec.Env)

	return spec
}

// countingWriter counts what passes through it, which is an archive's size.
type countingWriter struct {
	writer  io.Writer
	written int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	w.written += int64(n)

	return n, err
}
