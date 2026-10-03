package vm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// In-memory stand-ins for vmhost's parts that behave like the real ones, for
// tests that drive VMs through whole lives rather than one call at a time: a
// hypervisor whose machines are records, each with an agent of its own that
// runs tasks that do nothing but what the test says; an image store that makes
// any reference an image; and a fabric that hands out addresses. None of them
// needs Linux, KVM or root.

// errGone is what a fake machine's agent answers once its machine is gone, as
// a vsock whose machine is gone answers nothing.
var errGone = errors.New("the machine's vsock does not answer")

// FakeHypervisor boots machines that are nothing but records. Each machine
// gets an agent of its own (FakeGuests), reached at its VsockPath.
type FakeHypervisor struct {
	guests *FakeGuests

	lock     sync.Mutex
	machines map[string]vm.Machine
	booted   []vm.MachineSpec
	failBoot error
	pid      int
}

var _ vm.Hypervisor = (*FakeHypervisor)(nil)

// NewFakeHypervisor boots machines whose agents guests makes.
func NewFakeHypervisor(guests *FakeGuests) *FakeHypervisor {
	return &FakeHypervisor{guests: guests, machines: make(map[string]vm.Machine)}
}

func (h *FakeHypervisor) Name() string {
	return "fake"
}

func (h *FakeHypervisor) Version() string {
	return "0.0.0"
}

// Boot starts a machine, and its agent.
func (h *FakeHypervisor) Boot(ctx context.Context, spec vm.MachineSpec) (vm.Machine, error) {
	h.lock.Lock()

	if h.failBoot != nil {
		err := h.failBoot
		h.lock.Unlock()

		return vm.Machine{}, err
	}

	if existing, found := h.machines[spec.ID]; found && existing.Running {
		h.lock.Unlock()

		return vm.Machine{}, fmt.Errorf("machine %s is running already", spec.ID)
	}

	h.pid++

	machine := vm.Machine{
		ID:        spec.ID,
		Running:   true,
		PID:       h.pid,
		UID:       spec.UID,
		VsockPath: fmt.Sprintf("fake://%s/%d/v.sock", spec.ID, h.pid),
		Dir:       "/fake/j/" + spec.ID + "/root",
		Cgroup:    "/workload-vm.slice/workload-vm-" + spec.ID + ".service",
		Unit:      "workload-vm-" + spec.ID + ".service",
	}

	h.machines[spec.ID] = machine
	h.booted = append(h.booted, spec)
	h.lock.Unlock()

	pid := machine.PID
	h.guests.start(machine.VsockPath, func() { h.halted(spec.ID, pid) })

	return machine, nil
}

// halted is a machine whose agent turned it off: its VMM ends, and the
// machine is left behind until it is terminated.
func (h *FakeHypervisor) halted(id string, pid int) {
	h.lock.Lock()
	defer h.lock.Unlock()

	if machine, found := h.machines[id]; found && machine.PID == pid {
		machine.Running = false
		h.machines[id] = machine
	}
}

func (h *FakeHypervisor) Machine(ctx context.Context, id string) (vm.Machine, error) {
	h.lock.Lock()
	defer h.lock.Unlock()

	machine, found := h.machines[id]
	if !found {
		return vm.Machine{}, vm.ErrNotFound
	}

	return machine, nil
}

func (h *FakeHypervisor) Machines(ctx context.Context) ([]vm.Machine, error) {
	h.lock.Lock()
	defer h.lock.Unlock()

	machines := make([]vm.Machine, 0, len(h.machines))
	for _, machine := range h.machines {
		machines = append(machines, machine)
	}

	slices.SortFunc(machines, func(a vm.Machine, b vm.Machine) int { return strings.Compare(a.ID, b.ID) })

	return machines, nil
}

// Terminate ends a machine, and its agent with it.
func (h *FakeHypervisor) Terminate(ctx context.Context, id string) error {
	h.lock.Lock()
	machine, found := h.machines[id]
	delete(h.machines, id)
	h.lock.Unlock()

	if found {
		h.guests.stop(machine.VsockPath)
	}

	return nil
}

// Crash is a machine's VMM ending on its own, taking its agent with it.
func (h *FakeHypervisor) Crash(id string) {
	h.lock.Lock()
	machine, found := h.machines[id]
	if found {
		machine.Running = false
		h.machines[id] = machine
	}
	h.lock.Unlock()

	if found {
		h.guests.stop(machine.VsockPath)
	}
}

// Leave is a machine an earlier vmhost booted, running with agent.
func (h *FakeHypervisor) Leave(machine vm.Machine, agent *FakeAgent) {
	h.lock.Lock()
	h.machines[machine.ID] = machine
	h.lock.Unlock()

	h.guests.put(machine.VsockPath, agent)
}

// FailBoot makes every boot from now on fail with err; nil boots again.
func (h *FakeHypervisor) FailBoot(err error) {
	h.lock.Lock()
	defer h.lock.Unlock()

	h.failBoot = err
}

// Booted is every machine booted, in order.
func (h *FakeHypervisor) Booted() []vm.MachineSpec {
	h.lock.Lock()
	defer h.lock.Unlock()

	return slices.Clone(h.booted)
}

// FakeRun is what a fake task does once it is started: what it writes, and,
// when Exits, the code it ends with at once. One that does not exit runs
// until it is stopped, killed, or told to end (FakeAgent.Exit).
type FakeRun struct {
	Lines    []string
	Exits    bool
	ExitCode int
}

// FakeScript says what each run of a fake task does, given which run of it
// this is in its machine, from one, and what it was started as.
type FakeScript func(run uint64, process guest.Process) FakeRun

// FakeGuests makes the clients of fake machines' agents.
type FakeGuests struct {
	lock   sync.Mutex
	agents map[string]*FakeAgent
	script FakeScript
}

var _ vm.GuestConnector = (*FakeGuests)(nil)

// NewFakeGuests makes agents whose tasks do what script says; nil runs every
// task until it is stopped.
func NewFakeGuests(script FakeScript) *FakeGuests {
	return &FakeGuests{agents: make(map[string]*FakeAgent), script: script}
}

// Connect is the agent at vsockPath, or one that never answers when nothing
// is there.
func (g *FakeGuests) Connect(vsockPath string) vm.GuestClient {
	if agent := g.Agent(vsockPath); agent != nil {
		return agent
	}

	agent := NewFakeAgent(nil)
	agent.stop()

	return agent
}

// Agent is the agent at vsockPath, if a machine there has one.
func (g *FakeGuests) Agent(vsockPath string) *FakeAgent {
	g.lock.Lock()
	defer g.lock.Unlock()

	return g.agents[vsockPath]
}

func (g *FakeGuests) start(vsockPath string, poweredOff func()) {
	agent := NewFakeAgent(g.script)
	agent.poweredOff = poweredOff

	g.put(vsockPath, agent)
}

func (g *FakeGuests) put(vsockPath string, agent *FakeAgent) {
	g.lock.Lock()
	defer g.lock.Unlock()

	g.agents[vsockPath] = agent
}

func (g *FakeGuests) stop(vsockPath string) {
	if agent := g.Agent(vsockPath); agent != nil {
		agent.stop()
	}
}

// FakeAgent is the agent inside a fake machine. It keeps the guest protocol's
// promises: runs are numbered, Wait answers a run that ended at once, output
// is numbered from one and kept, and a machine that is gone answers nothing.
type FakeAgent struct {
	script     FakeScript
	poweredOff func()

	lock     sync.Mutex
	gone     bool
	goneCh   chan struct{}
	refusal  error
	config   *guest.Config
	hosts    []guest.Host
	status   guest.Status
	ended    chan struct{}
	lines    []guest.LogLine
	changed  chan struct{}
	execs    map[string]bool
	execSeq  int
	stats    guest.Stats
	closed   int
	shutdown bool
}

var _ vm.GuestClient = (*FakeAgent)(nil)

// NewFakeAgent is an agent whose tasks do what script says.
func NewFakeAgent(script FakeScript) *FakeAgent {
	return &FakeAgent{
		script:  script,
		goneCh:  make(chan struct{}),
		status:  guest.Status{State: guest.StateCreated},
		ended:   make(chan struct{}),
		changed: make(chan struct{}),
		execs:   make(map[string]bool),
	}
}

// Running is an agent whose task already runs, as one a machine left running
// by an earlier vmhost has: configured, on run generation, having written
// lines.
func (a *FakeAgent) Running(generation uint64, lines ...string) *FakeAgent {
	a.lock.Lock()
	defer a.lock.Unlock()

	a.config = &guest.Config{}
	a.status = guest.Status{State: guest.StateRunning, Generation: generation, StartedAt: time.Now().UTC()}

	for _, line := range lines {
		a.write(guest.StreamStdout, line)
	}

	return a
}

func (a *FakeAgent) stop() {
	a.lock.Lock()
	defer a.lock.Unlock()

	if !a.gone {
		a.gone = true
		close(a.goneCh)
	}
}

// Hang is an agent that stops answering while its machine's VMM keeps
// running: a guest that is stuck.
func (a *FakeAgent) Hang() {
	a.stop()
}

// Refuse makes Ready refuse with err, as a client refuses an agent whose
// protocol it does not know.
func (a *FakeAgent) Refuse(err error) {
	a.lock.Lock()
	defer a.lock.Unlock()

	a.refusal = err
}

// Exit ends the task's current run with code, as a task that ends on its own.
func (a *FakeAgent) Exit(code int) {
	a.lock.Lock()
	defer a.lock.Unlock()

	a.end(code)
}

// Write is the task writing a line.
func (a *FakeAgent) Write(stream string, content string) {
	a.lock.Lock()
	defer a.lock.Unlock()

	a.write(stream, content)
}

// Configured is what the machine was told it is, if it was.
func (a *FakeAgent) Configured() *guest.Config {
	a.lock.Lock()
	defer a.lock.Unlock()

	return a.config
}

// Hosts are the names the machine was last told its neighbours answer to.
func (a *FakeAgent) Hosts() []guest.Host {
	a.lock.Lock()
	defer a.lock.Unlock()

	return slices.Clone(a.hosts)
}

// SetStats is what the guest says its task uses.
func (a *FakeAgent) SetStats(stats guest.Stats) {
	a.lock.Lock()
	defer a.lock.Unlock()

	a.stats = stats
}

// PoweredOff says the machine was asked to turn itself off.
func (a *FakeAgent) PoweredOff() bool {
	a.lock.Lock()
	defer a.lock.Unlock()

	return a.shutdown
}

// Gone says the machine is gone.
func (a *FakeAgent) Gone() bool {
	a.lock.Lock()
	defer a.lock.Unlock()

	return a.gone
}

// end ends the current run with code. The lock is held.
func (a *FakeAgent) end(code int) {
	if a.status.State != guest.StateRunning {
		return
	}

	a.status.State = guest.StateExited
	a.status.ExitCode = code
	a.status.FinishedAt = time.Now().UTC()
	close(a.ended)
}

// write keeps a line of output. The lock is held.
func (a *FakeAgent) write(stream string, content string) {
	a.lines = append(a.lines, guest.LogLine{
		Seq:     uint64(len(a.lines) + 1),
		Stream:  stream,
		At:      time.Now().UTC(),
		Content: content,
	})

	close(a.changed)
	a.changed = make(chan struct{})
}

func (a *FakeAgent) Ready(ctx context.Context) error {
	a.lock.Lock()
	gone, refusal := a.gone, a.refusal
	a.lock.Unlock()

	if refusal != nil {
		return refusal
	}

	if gone {
		<-ctx.Done()

		return errors.Join(ctx.Err(), errGone)
	}

	return nil
}

func (a *FakeAgent) Configure(ctx context.Context, config guest.Config) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	if a.gone {
		return errGone
	}

	a.config = &config
	a.hosts = config.Hosts

	return nil
}

func (a *FakeAgent) SetHosts(ctx context.Context, hosts []guest.Host) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	if a.gone {
		return errGone
	}

	a.hosts = hosts

	return nil
}

func (a *FakeAgent) Start(ctx context.Context, process guest.Process) (guest.Status, error) {
	a.lock.Lock()
	defer a.lock.Unlock()

	switch {
	case a.gone:
		return guest.Status{}, errGone
	case a.config == nil:
		return guest.Status{}, fmt.Errorf("%w: the machine has not been told what it is", guest.ErrNotRunning)
	case a.status.State == guest.StateRunning:
		return guest.Status{}, fmt.Errorf("%w: the task is already running", guest.ErrNotRunning)
	}

	a.status = guest.Status{State: guest.StateRunning, Generation: a.status.Generation + 1, StartedAt: time.Now().UTC()}
	a.ended = make(chan struct{})

	started := a.status

	var run FakeRun
	if a.script != nil {
		run = a.script(started.Generation, process)
	}

	for _, line := range run.Lines {
		a.write(guest.StreamStdout, line)
	}

	if run.Exits {
		a.end(run.ExitCode)
	}

	return started, nil
}

func (a *FakeAgent) Status(ctx context.Context) (guest.Status, error) {
	a.lock.Lock()
	defer a.lock.Unlock()

	if a.gone {
		return guest.Status{}, errGone
	}

	return a.status, nil
}

func (a *FakeAgent) Wait(ctx context.Context, generation uint64) (guest.Status, error) {
	for {
		a.lock.Lock()
		gone, status, ended := a.gone, a.status, a.ended
		a.lock.Unlock()

		switch {
		case gone:
			return guest.Status{}, errGone
		case status.Generation > generation || status.State == guest.StateExited:
			return status, nil
		case status.State == guest.StateCreated:
			return guest.Status{}, fmt.Errorf("%w: the task has not been started", guest.ErrNotRunning)
		}

		select {
		case <-ended:
		case <-a.goneCh:
		case <-ctx.Done():
			return guest.Status{}, ctx.Err()
		}
	}
}

func (a *FakeAgent) Signal(ctx context.Context, signal int) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	switch {
	case a.gone:
		return errGone
	case a.status.State != guest.StateRunning:
		return fmt.Errorf("%w: the task is not running", guest.ErrNotRunning)
	}

	a.end(128 + signal)

	return nil
}

func (a *FakeAgent) Stop(ctx context.Context, timeout time.Duration) (guest.Status, error) {
	a.lock.Lock()
	defer a.lock.Unlock()

	if a.gone {
		return guest.Status{}, errGone
	}

	a.end(128 + int(syscall.SIGTERM))

	return a.status, nil
}

func (a *FakeAgent) Stats(ctx context.Context) (guest.Stats, error) {
	a.lock.Lock()
	defer a.lock.Unlock()

	if a.gone {
		return guest.Stats{}, errGone
	}

	return a.stats, nil
}

// PowerOff turns the machine off: its agent is gone, and its VMM ends.
func (a *FakeAgent) PowerOff(ctx context.Context) error {
	a.lock.Lock()
	if a.gone {
		a.lock.Unlock()

		return errGone
	}

	a.shutdown = true
	a.gone = true
	close(a.goneCh)
	poweredOff := a.poweredOff
	a.lock.Unlock()

	if poweredOff != nil {
		poweredOff()
	}

	return nil
}

func (a *FakeAgent) Logs(ctx context.Context, after uint64, follow bool, emit func(guest.LogLine) error) error {
	for {
		a.lock.Lock()
		gone, changed := a.gone, a.changed
		var pending []guest.LogLine
		for _, line := range a.lines {
			if line.Seq > after {
				pending = append(pending, line)
			}
		}
		a.lock.Unlock()

		if gone && len(pending) == 0 {
			return errGone
		}

		for _, line := range pending {
			if err := emit(line); err != nil {
				return err
			}

			after = line.Seq
		}

		if !follow {
			return nil
		}

		select {
		case <-changed:
		case <-a.goneCh:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Exec runs a command that echoes what it is given: every frame of input
// comes back as output, a resize comes back as "rows cols", and closing its
// input ends it with 0.
func (a *FakeAgent) Exec(ctx context.Context, exec guest.Exec) (string, net.Conn, error) {
	a.lock.Lock()

	switch {
	case a.gone:
		a.lock.Unlock()

		return "", nil, errGone
	case a.status.State != guest.StateRunning:
		a.lock.Unlock()

		return "", nil, fmt.Errorf("%w: the task is not running", guest.ErrNotRunning)
	}

	a.execSeq++
	id := fmt.Sprintf("exec-%d", a.execSeq)
	a.execs[id] = true
	a.lock.Unlock()

	client, server := net.Pipe()

	go func() {
		defer server.Close()

		for {
			frame, err := guest.ReadFrame(server)
			if err != nil {
				return
			}

			switch frame.Type {
			case guest.FrameStdin:
				if err := guest.WriteFrame(server, guest.FrameStdout, frame.Payload); err != nil {
					return
				}
			case guest.FrameResize:
				rows, cols, err := guest.ParseResize(frame.Payload)
				if err != nil {
					return
				}

				if err := guest.WriteFrame(server, guest.FrameStdout, fmt.Appendf(nil, "%d %d", rows, cols)); err != nil {
					return
				}
			case guest.FrameCloseStdin:
				_ = guest.WriteFrame(server, guest.FrameExit, guest.ExitPayload(0))

				return
			}
		}
	}()

	return id, client, nil
}

func (a *FakeAgent) EndExec(ctx context.Context, id string, end guest.EndExec) (guest.Ended, error) {
	a.lock.Lock()
	defer a.lock.Unlock()

	if a.gone {
		return guest.Ended{}, errGone
	}

	signalled := a.execs[id]
	delete(a.execs, id)

	return guest.Ended{Signalled: signalled}, nil
}

// Dial connects to a task port that echoes what it is sent.
func (a *FakeAgent) Dial(ctx context.Context, port uint16) (net.Conn, error) {
	a.lock.Lock()
	gone, running := a.gone, a.status.State == guest.StateRunning
	a.lock.Unlock()

	switch {
	case gone:
		return nil, errGone
	case !running:
		return nil, fmt.Errorf("%w: the task is not running", guest.ErrNotRunning)
	}

	client, server := net.Pipe()

	go func() {
		defer server.Close()

		_, _ = io.Copy(server, server)
	}()

	return client, nil
}

// Close counts the times its client was let go of.
func (a *FakeAgent) Close() error {
	a.lock.Lock()
	defer a.lock.Unlock()

	a.closed++

	return nil
}

// FakeImageStore makes any reference an image: one with /bin/sh to run,
// unless the test said otherwise (Add).
type FakeImageStore struct {
	lock    sync.Mutex
	images  map[string]vm.Image
	refs    map[string]string
	scratch map[string]uint64
	fail    error
}

var _ vm.ImageStore = (*FakeImageStore)(nil)

func NewFakeImageStore() *FakeImageStore {
	return &FakeImageStore{
		images:  make(map[string]vm.Image),
		refs:    make(map[string]string),
		scratch: make(map[string]uint64),
	}
}

// Add is image, as reference names it.
func (s *FakeImageStore) Add(reference string, image vm.Image) {
	s.lock.Lock()
	defer s.lock.Unlock()

	image.Reference = reference
	s.images[image.Digest] = image
	s.refs[reference] = image.Digest
}

// Fail makes every Ensure from now on fail with err; nil makes images again.
func (s *FakeImageStore) Fail(err error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.fail = err
}

// Scratch is the size of the scratch disk made at path, if one was.
func (s *FakeImageStore) Scratch(path string) (uint64, bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	size, found := s.scratch[path]

	return size, found
}

func (s *FakeImageStore) Ensure(ctx context.Context, reference string) (vm.Image, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.fail != nil {
		return vm.Image{}, s.fail
	}

	digest, found := s.refs[reference]
	if !found {
		sum := sha256.Sum256([]byte(reference))
		digest = "sha256:" + hex.EncodeToString(sum[:])

		s.images[digest] = vm.Image{
			Reference: reference,
			Digest:    digest,
			Root:      "/fake/images/" + digest + "/rootfs.squashfs",
			Size:      1 << 20,
			Config:    vm.ImageConfig{Cmd: []string{"/bin/sh"}, Env: []string{"PATH=/usr/bin:/bin"}},
		}
		s.refs[reference] = digest
	}

	image := s.images[digest]
	image.LastUsedAt = time.Now().UTC()
	s.images[digest] = image

	return image, nil
}

func (s *FakeImageStore) List(ctx context.Context) ([]vm.Image, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	images := make([]vm.Image, 0, len(s.images))
	for _, image := range s.images {
		images = append(images, image)
	}

	slices.SortFunc(images, func(a vm.Image, b vm.Image) int { return strings.Compare(a.Digest, b.Digest) })

	return images, nil
}

func (s *FakeImageStore) Remove(ctx context.Context, digest string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if _, found := s.images[digest]; !found {
		return vm.ErrNotFound
	}

	delete(s.images, digest)

	for reference, kept := range s.refs {
		if kept == digest {
			delete(s.refs, reference)
		}
	}

	return nil
}

func (s *FakeImageStore) MakeScratch(ctx context.Context, path string, size uint64) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.scratch[path] = size

	return nil
}

// FakeFabric hands out a /24 for each network and an address for each tap.
type FakeFabric struct {
	lock      sync.Mutex
	networks  map[string]vm.Network
	order     []string
	hosts     map[string]int
	plugs     map[string][]vm.Interface
	retained  []string
	repairs   int
	repairErr error
}

var _ vm.Fabric = (*FakeFabric)(nil)

func NewFakeFabric() *FakeFabric {
	return &FakeFabric{
		networks: make(map[string]vm.Network),
		hosts:    make(map[string]int),
		plugs:    make(map[string][]vm.Interface),
	}
}

// FailRepair makes every repair from now on fail with err; nil repairs again.
func (f *FakeFabric) FailRepair(err error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.repairErr = err
}

// Plugged is what a machine is plugged into.
func (f *FakeFabric) Plugged(id string) []vm.Interface {
	f.lock.Lock()
	defer f.lock.Unlock()

	return slices.Clone(f.plugs[id])
}

// Retained is what the last Retain was told to keep.
func (f *FakeFabric) Retained() []string {
	f.lock.Lock()
	defer f.lock.Unlock()

	return slices.Clone(f.retained)
}

// Repairs is how many times the firewall was put back.
func (f *FakeFabric) Repairs() int {
	f.lock.Lock()
	defer f.lock.Unlock()

	return f.repairs
}

func (f *FakeFabric) EnsureNetwork(ctx context.Context, name string, masquerade bool) (vm.Network, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	if network, found := f.networks[name]; found {
		if network.Masquerade != masquerade {
			return vm.Network{}, fmt.Errorf("%w: network %s routes out: %t", vm.ErrConflict, name, network.Masquerade)
		}

		return network, nil
	}

	n := len(f.order)
	network := vm.Network{
		Name:       name,
		Subnet:     fmt.Sprintf("10.250.%d.0/24", n),
		Gateway:    fmt.Sprintf("10.250.%d.1", n),
		Masquerade: masquerade,
	}

	f.networks[name] = network
	f.order = append(f.order, name)

	return network, nil
}

func (f *FakeFabric) RemoveNetwork(ctx context.Context, name string) error {
	f.lock.Lock()
	defer f.lock.Unlock()

	for _, interfaces := range f.plugs {
		for _, i := range interfaces {
			if i.Network == name {
				return fmt.Errorf("%w: %s", vm.ErrNetworkInUse, name)
			}
		}
	}

	delete(f.networks, name)
	f.order = slices.DeleteFunc(f.order, func(n string) bool { return n == name })

	return nil
}

func (f *FakeFabric) Networks(ctx context.Context) ([]vm.Network, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	networks := make([]vm.Network, 0, len(f.order))
	for _, name := range f.order {
		networks = append(networks, f.networks[name])
	}

	return networks, nil
}

func (f *FakeFabric) Plug(ctx context.Context, id string, uid int, attachments []vm.Attachment) ([]vm.Interface, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	interfaces := make([]vm.Interface, 0, len(attachments))

	for i, attachment := range attachments {
		network, found := f.networks[attachment.Network]
		if !found {
			return nil, fmt.Errorf("%w: no network %s", vm.ErrNotFound, attachment.Network)
		}

		f.hosts[network.Name]++
		host := f.hosts[network.Name] + 1

		prefix, _, _ := strings.Cut(network.Subnet, ".0/")

		plugged := vm.Interface{
			Network: network.Name,
			Device:  fmt.Sprintf("wkt%s%d", id[:min(8, len(id))], i),
			MAC:     fmt.Sprintf("06:00:0a:fa:%02x:%02x", len(f.hosts)%256, host%256),
			Address: fmt.Sprintf("%s.%d/24", prefix, host),
			Aliases: slices.Clone(attachment.Aliases),
		}

		if attachment.Gateway {
			plugged.Gateway = network.Gateway
		}

		interfaces = append(interfaces, plugged)
	}

	f.plugs[id] = interfaces

	return slices.Clone(interfaces), nil
}

func (f *FakeFabric) Unplug(ctx context.Context, id string) error {
	f.lock.Lock()
	defer f.lock.Unlock()

	delete(f.plugs, id)

	return nil
}

func (f *FakeFabric) Retain(ctx context.Context, ids []string) error {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.retained = slices.Clone(ids)

	for id := range f.plugs {
		if !slices.Contains(ids, id) {
			delete(f.plugs, id)
		}
	}

	return nil
}

func (f *FakeFabric) Repair(ctx context.Context) error {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.repairs++

	return f.repairErr
}
