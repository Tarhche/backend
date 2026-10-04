// Package microsandbox stands in for microsandbox and for the
// workload-microsandbox service's stores, as the run supervisor sees them.
//
// FakeSandboxes is microsandbox in memory: sandboxes that boot at once, and
// processes a test drives — writing, exiting, dying of a signal, or losing
// their VM — with signals that behave as agentd's do. The Memory* types are
// stores that keep everything in memory, and the Mock* types are testify mocks
// of the port and the stores, for what a fake cannot easily be made to do.
package microsandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
)

// FakeVersions are the versions a FakeSandboxes reports unless told otherwise:
// an SDK and an msb of one version, as microsandbox's own image pairs them.
var FakeVersions = runs.Versions{SDK: "0.7.6", Runtime: "msb 0.7.6", Architecture: "amd64"}

// FakeSandboxes is microsandbox in memory.
//
// Sandboxes boot as soon as they are made or started, and the first command
// started in a sandbox after each boot is its main process. Processes start at
// once, unless the program is one SpawnFails was told of, and then run until
// the test ends them or something does to them what would end a real one: a
// signal they do not trap, their handle being closed, or their VM being
// stopped or destroyed.
type FakeSandboxes struct {
	mu sync.Mutex

	versions runs.Versions
	checkErr error

	registry map[string]runs.ImageConfig
	cached   map[string]bool
	pullErrs map[string]error
	pulls    map[string]int

	vms map[string]*fakeVM

	spawnFailures map[string]string
	metrics       map[string]runs.Metrics

	createErr   error
	createLeave bool
	startErr    error
	listErr     error
	metricsErr  error

	onExec func(*FakeProcess)

	sweeps []Sweep
	calls  []string
}

var _ runs.Sandboxes = &FakeSandboxes{}

type fakeVM struct {
	spec    runs.SandboxSpec
	labels  map[string]string
	running bool
	boots   int

	processes []*FakeProcess

	// mains are the main processes, one for each boot.
	mains []*FakeProcess

	stops int
}

// Sweep is a sweep the supervisor ran inside a sandbox, to signal everything
// an exec left behind.
type Sweep struct {
	Sandbox string
	Exec    string
	Signal  syscall.Signal
}

// NewFakeSandboxes is a microsandbox with no images and no sandboxes, whose
// runtime checks out.
func NewFakeSandboxes() *FakeSandboxes {
	return &FakeSandboxes{
		versions:      FakeVersions,
		registry:      make(map[string]runs.ImageConfig),
		cached:        make(map[string]bool),
		pullErrs:      make(map[string]error),
		pulls:         make(map[string]int),
		vms:           make(map[string]*fakeVM),
		spawnFailures: make(map[string]string),
		metrics:       make(map[string]runs.Metrics),
	}
}

// SetVersions is what Check reports from then on, and SetCheckError what it
// fails with.
func (f *FakeSandboxes) SetVersions(versions runs.Versions) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.versions = versions
}

func (f *FakeSandboxes) SetCheckError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.checkErr = err
}

// AddImage puts an image in the registry, where Pull finds it.
func (f *FakeSandboxes) AddImage(reference string, config runs.ImageConfig) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.registry[reference] = config
}

// CacheImage puts an image in the registry and in the cache.
func (f *FakeSandboxes) CacheImage(reference string, config runs.ImageConfig) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.registry[reference] = config
	f.cached[reference] = true
}

// FailPull is what pulling an image fails with.
func (f *FakeSandboxes) FailPull(reference string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.pullErrs[reference] = err
}

// Pulls is how many times an image was pulled.
func (f *FakeSandboxes) Pulls(reference string) int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.pulls[reference]
}

// SpawnFails makes a program fail to start with errno, as a program that is
// not there fails with ENOENT.
func (f *FakeSandboxes) SpawnFails(program, errno string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.spawnFailures[program] = errno
}

// FailCreate is what making a sandbox fails with. With leave, the failed make
// leaves a stopped sandbox behind, as cancelling microsandbox's does.
func (f *FakeSandboxes) FailCreate(err error, leave bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.createErr, f.createLeave = err, leave
}

// FailStart is what booting a stopped sandbox fails with.
func (f *FakeSandboxes) FailStart(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.startErr = err
}

// FailList is what listing sandboxes fails with.
func (f *FakeSandboxes) FailList(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.listErr = err
}

// FailMetrics is what reading metrics fails with.
func (f *FakeSandboxes) FailMetrics(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.metricsErr = err
}

// SetMetrics is what a sandbox reports using.
func (f *FakeSandboxes) SetMetrics(name string, metrics runs.Metrics) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.metrics[name] = metrics
}

// OnExec is called with every process that starts, but sweeps, once it has
// started, so a test can script what it does.
func (f *FakeSandboxes) OnExec(script func(*FakeProcess)) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.onExec = script
}

// Seed puts a sandbox in place, as one a service that went away left behind.
func (f *FakeSandboxes) Seed(name string, labels map[string]string, running bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.vms[name] = &fakeVM{
		spec:    runs.SandboxSpec{Name: name, Labels: maps.Clone(labels)},
		labels:  maps.Clone(labels),
		running: running,
		boots:   1,
	}
}

// Exists is whether a sandbox is there, and Running whether it is up.
func (f *FakeSandboxes) Exists(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	_, exists := f.vms[name]

	return exists
}

func (f *FakeSandboxes) Running(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	vm, exists := f.vms[name]

	return exists && vm.running
}

// Spec is what a sandbox was made with.
func (f *FakeSandboxes) Spec(name string) (runs.SandboxSpec, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	vm, exists := f.vms[name]
	if !exists {
		return runs.SandboxSpec{}, false
	}

	return vm.spec, true
}

// Boots is how many times a sandbox has booted, and Stops how many times it
// has been stopped.
func (f *FakeSandboxes) Boots(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()

	if vm, exists := f.vms[name]; exists {
		return vm.boots
	}

	return 0
}

func (f *FakeSandboxes) Stops(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()

	if vm, exists := f.vms[name]; exists {
		return vm.stops
	}

	return 0
}

// Main is a sandbox's main process of a boot, counted from 1, and false while
// that boot has none.
func (f *FakeSandboxes) Main(name string, boot int) (*FakeProcess, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	vm, exists := f.vms[name]
	if !exists || boot < 1 || boot > len(vm.mains) {
		return nil, false
	}

	return vm.mains[boot-1], true
}

// Processes are every command started in a sandbox, sweeps included, in the
// order they were started.
func (f *FakeSandboxes) Processes(name string) []*FakeProcess {
	f.mu.Lock()
	defer f.mu.Unlock()

	if vm, exists := f.vms[name]; exists {
		return slices.Clone(vm.processes)
	}

	return nil
}

// Sweeps are the sweeps run, in order.
func (f *FakeSandboxes) Sweeps() []Sweep {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.sweeps)
}

// Calls are the calls made, by name, in order.
func (f *FakeSandboxes) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.calls)
}

func (f *FakeSandboxes) Check(ctx context.Context) (runs.Versions, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, "check")

	return f.versions, f.checkErr
}

func (f *FakeSandboxes) Pull(ctx context.Context, reference string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, "pull "+reference)
	f.pulls[reference]++

	if err := f.pullErrs[reference]; err != nil {
		return err
	}

	if _, found := f.registry[reference]; !found {
		return fmt.Errorf("pull access denied for %s: manifest unknown", reference)
	}

	f.cached[reference] = true

	return nil
}

func (f *FakeSandboxes) Image(ctx context.Context, reference string) (runs.ImageConfig, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.cached[reference] {
		return runs.ImageConfig{}, false, nil
	}

	return f.registry[reference], true, nil
}

func (f *FakeSandboxes) Create(ctx context.Context, spec runs.SandboxSpec) (runs.Sandbox, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, "create "+spec.Name)

	if f.createErr != nil {
		if f.createLeave {
			f.vms[spec.Name] = &fakeVM{spec: spec, labels: maps.Clone(spec.Labels)}
		}

		return nil, f.createErr
	}

	if _, exists := f.vms[spec.Name]; exists {
		return nil, fmt.Errorf("a sandbox named %s already exists", spec.Name)
	}

	if !f.cached[spec.Image] {
		return nil, fmt.Errorf("image %s is not cached", spec.Image)
	}

	spec.Env = maps.Clone(spec.Env)
	spec.Labels = maps.Clone(spec.Labels)
	spec.Ports = slices.Clone(spec.Ports)
	spec.Nameservers = slices.Clone(spec.Nameservers)

	f.vms[spec.Name] = &fakeVM{spec: spec, labels: maps.Clone(spec.Labels), running: true, boots: 1}

	return &fakeSandbox{fake: f, name: spec.Name}, nil
}

func (f *FakeSandboxes) Start(ctx context.Context, name string) (runs.Sandbox, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, "start "+name)

	if f.startErr != nil {
		return nil, f.startErr
	}

	vm, exists := f.vms[name]
	if !exists {
		return nil, fmt.Errorf("there is no sandbox named %s", name)
	}

	if vm.running {
		return nil, fmt.Errorf("sandbox %s is already running", name)
	}

	vm.running = true
	vm.boots++

	return &fakeSandbox{fake: f, name: name}, nil
}

func (f *FakeSandboxes) Connect(ctx context.Context, name string) (runs.Sandbox, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	vm, exists := f.vms[name]
	if !exists || !vm.running {
		return nil, fmt.Errorf("sandbox %s is not running", name)
	}

	return &fakeSandbox{fake: f, name: name}, nil
}

// Stop stops a sandbox, which ends every command in it.
func (f *FakeSandboxes) Stop(ctx context.Context, name string, timeout time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, "stop "+name)

	vm, exists := f.vms[name]
	if !exists {
		return nil
	}

	vm.stops++
	vm.running = false

	for _, process := range vm.processes {
		process.lose()
	}

	return nil
}

// Remove destroys a sandbox, which ends every command in it.
func (f *FakeSandboxes) Remove(ctx context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, "remove "+name)

	vm, exists := f.vms[name]
	if !exists {
		return nil
	}

	for _, process := range vm.processes {
		process.lose()
	}

	delete(f.vms, name)

	return nil
}

func (f *FakeSandboxes) List(ctx context.Context, labels map[string]string) ([]runs.SandboxInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.listErr != nil {
		return nil, f.listErr
	}

	var infos []runs.SandboxInfo

	for name, vm := range f.vms {
		matches := true

		for key, value := range labels {
			if vm.labels[key] != value {
				matches = false

				break
			}
		}

		if matches {
			infos = append(infos, runs.SandboxInfo{Name: name, Labels: maps.Clone(vm.labels), Running: vm.running})
		}
	}

	slices.SortFunc(infos, func(a, b runs.SandboxInfo) int { return strings.Compare(a.Name, b.Name) })

	return infos, nil
}

func (f *FakeSandboxes) Metrics(ctx context.Context) (map[string]runs.Metrics, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, "metrics")

	if f.metricsErr != nil {
		return nil, f.metricsErr
	}

	metrics := make(map[string]runs.Metrics)

	for name, vm := range f.vms {
		if !vm.running {
			continue
		}

		if reported, found := f.metrics[name]; found {
			metrics[name] = reported

			continue
		}

		metrics[name] = runs.Metrics{MemoryLimit: vm.spec.Memory}
	}

	return metrics, nil
}

// fakeSandbox is a handle on a running sandbox. Closing it only lets go of
// it, as closing a detached sandbox's handle does.
type fakeSandbox struct {
	fake *FakeSandboxes
	name string
}

func (s *fakeSandbox) Name() string {
	return s.name
}

func (s *fakeSandbox) Close() error {
	return nil
}

func (s *fakeSandbox) Exec(ctx context.Context, command runs.Command) (runs.Process, error) {
	f := s.fake

	f.mu.Lock()

	vm, exists := f.vms[s.name]
	if !exists || !vm.running {
		f.mu.Unlock()

		return nil, fmt.Errorf("sandbox %s is not running", s.name)
	}

	process := newFakeProcess(s.name, command)
	vm.processes = append(vm.processes, process)

	sweep, isSweep := sweepOf(s.name, command)

	if !isSweep && len(vm.mains) < vm.boots {
		vm.mains = append(vm.mains, process)
	}

	errno, fails := "", false
	if len(command.Argv) > 0 {
		errno, fails = f.spawnFailures[command.Argv[0]]
	}

	var swept []*FakeProcess

	if isSweep {
		f.sweeps = append(f.sweeps, sweep)

		for _, other := range vm.processes {
			if slices.Contains(other.command.Env, "WORKLOAD_TERMINAL_SESSION="+sweep.Exec) {
				swept = append(swept, other)
			}
		}
	}

	script := f.onExec

	f.mu.Unlock()

	switch {
	case fails:
		process.end(runs.Event{
			Kind:    runs.EventFailed,
			Errno:   errno,
			Message: fmt.Sprintf("spawn %q: %s", command.Argv[0], errno),
		})
	case isSweep:
		process.emit(runs.Event{Kind: runs.EventStarted})

		found := false
		for _, other := range swept {
			if other.Signal(ctx, sweep.Signal) == nil {
				found = true
			}
		}

		code := 1
		if found {
			code = 0
		}

		process.Exit(code)
	default:
		process.emit(runs.Event{Kind: runs.EventStarted})

		if script != nil {
			script(process)
		}
	}

	return process, nil
}

var (
	sweepMarker = regexp.MustCompile(`WORKLOAD_TERMINAL_SESSION=([0-9A-Za-z_-]+)`)
	sweepSignal = regexp.MustCompile(`kill -([0-9]+)`)
)

// sweepOf reads a sweep from the command the supervisor runs for one.
func sweepOf(sandbox string, command runs.Command) (Sweep, bool) {
	if len(command.Argv) != 3 || command.Argv[0] != "/bin/sh" || command.Argv[1] != "-c" {
		return Sweep{}, false
	}

	marker := sweepMarker.FindStringSubmatch(command.Argv[2])
	signal := sweepSignal.FindStringSubmatch(command.Argv[2])

	if marker == nil || signal == nil {
		return Sweep{}, false
	}

	number, _ := strconv.Atoi(signal[1])

	return Sweep{Sandbox: sandbox, Exec: marker[1], Signal: syscall.Signal(number)}, true
}

// FakeProcess is a command running in a fake sandbox.
//
// A signal it does not trap ends it as a signal ends a process in a guest,
// with an exit code of -1; one it traps ends it with the code it was told to
// exit with, and one it ignores does nothing. SIGKILL can be neither trapped
// nor ignored. A deaf process takes no notice of any
// signal, as one whose guest agent stopped answering would, and ends only with
// its VM or when its handle is closed.
type FakeProcess struct {
	sandbox string
	command runs.Command

	events chan runs.Event
	stdin  *fakeStdin

	mu      sync.Mutex
	ended   bool
	closed  bool
	deaf    bool
	traps   map[syscall.Signal]int
	ignored map[syscall.Signal]bool
	signals []syscall.Signal
	resizes [][2]uint16
}

var _ runs.Process = &FakeProcess{}

func newFakeProcess(sandbox string, command runs.Command) *FakeProcess {
	command.Argv = slices.Clone(command.Argv)
	command.Env = slices.Clone(command.Env)

	process := &FakeProcess{
		sandbox: sandbox,
		command: command,
		events:  make(chan runs.Event, 4096),
		traps:   make(map[syscall.Signal]int),
		ignored: make(map[syscall.Signal]bool),
	}

	if command.Stdin {
		process.stdin = &fakeStdin{}
	}

	return process
}

// Command is what the process was started as.
func (p *FakeProcess) Command() runs.Command {
	return p.command
}

func (p *FakeProcess) Events() <-chan runs.Event {
	return p.events
}

func (p *FakeProcess) Stdin() io.WriteCloser {
	if p.stdin == nil {
		return nil
	}

	return p.stdin
}

func (p *FakeProcess) Signal(ctx context.Context, signal syscall.Signal) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.signals = append(p.signals, signal)

	if p.ended {
		return errors.New("no such process")
	}

	if p.deaf || (p.ignored[signal] && signal != syscall.Signal(9)) {
		return nil
	}

	if code, trapped := p.traps[signal]; trapped && signal != syscall.Signal(9) {
		p.finish(runs.Event{Kind: runs.EventExited, ExitCode: code})

		return nil
	}

	p.finish(runs.Event{Kind: runs.EventExited, ExitCode: -1})

	return nil
}

func (p *FakeProcess) Resize(ctx context.Context, rows, cols uint16) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.resizes = append(p.resizes, [2]uint16{rows, cols})

	return nil
}

// Close lets go of the process, and ends it if it is still running, as
// closing microsandbox's exec handle does.
func (p *FakeProcess) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.closed = true
	p.finish(runs.Event{Kind: runs.EventExited, ExitCode: -1})

	return nil
}

// Write is the process writing to its stdout, and WriteErr to its stderr.
func (p *FakeProcess) Write(output string) {
	p.emit(runs.Event{Kind: runs.EventStdout, Data: []byte(output)})
}

func (p *FakeProcess) WriteErr(output string) {
	p.emit(runs.Event{Kind: runs.EventStderr, Data: []byte(output)})
}

// Exit is the process exiting by itself with code.
func (p *FakeProcess) Exit(code int) {
	p.end(runs.Event{Kind: runs.EventExited, ExitCode: code})
}

// Die is the process dying of a signal nobody sent, as the guest's OOM killer
// kills one.
func (p *FakeProcess) Die() {
	p.end(runs.Event{Kind: runs.EventExited, ExitCode: -1})
}

// Lose is the process's stream ending without an exit, as it does when its VM
// dies.
func (p *FakeProcess) Lose() {
	p.end(runs.Event{Kind: runs.EventLost})
}

// Trap has the process exit with code when it is sent signal.
func (p *FakeProcess) Trap(signal syscall.Signal, code int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.traps[signal] = code
}

// Ignore has the process take no notice of signal. SIGKILL cannot be ignored.
func (p *FakeProcess) Ignore(signal syscall.Signal) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.ignored[signal] = true
}

// Deafen has the process take no notice of signals.
func (p *FakeProcess) Deafen() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.deaf = true
}

// Signals are the signals the process was sent, in order.
func (p *FakeProcess) Signals() []syscall.Signal {
	p.mu.Lock()
	defer p.mu.Unlock()

	return slices.Clone(p.signals)
}

// Resizes are the sizes its terminal was given, in order, as rows and
// columns.
func (p *FakeProcess) Resizes() [][2]uint16 {
	p.mu.Lock()
	defer p.mu.Unlock()

	return slices.Clone(p.resizes)
}

// Stdin is what was written to the process's standard input, and whether it
// was closed.
func (p *FakeProcess) StdinData() (string, bool) {
	if p.stdin == nil {
		return "", false
	}

	return p.stdin.data()
}

// Ended is whether the process has ended, and Closed whether its handle has
// been closed.
func (p *FakeProcess) Ended() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.ended
}

func (p *FakeProcess) Closed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.closed
}

func (p *FakeProcess) emit(event runs.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.ended {
		p.events <- event
	}
}

func (p *FakeProcess) end(event runs.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.finish(event)
}

// lose ends the process with its VM.
func (p *FakeProcess) lose() {
	p.end(runs.Event{Kind: runs.EventLost})
}

// finish sends the process's last event and closes its events. Called with mu
// held.
func (p *FakeProcess) finish(event runs.Event) {
	if p.ended {
		return
	}

	p.ended = true
	p.events <- event
	close(p.events)
}

// fakeStdin is a process's standard input, which keeps what is written to it.
type fakeStdin struct {
	mu      sync.Mutex
	written strings.Builder
	closed  bool
}

func (s *fakeStdin) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return 0, io.ErrClosedPipe
	}

	return s.written.Write(p)
}

func (s *fakeStdin) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true

	return nil
}

func (s *fakeStdin) data() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.written.String(), s.closed
}
