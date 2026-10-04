package runs

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

// startCause is why a run is being started, which decides what becomes of it
// when it cannot be.
type startCause int

const (
	// causeRequest is a client asking. A start that fails before anything
	// ran leaves the run as it was, and the client is told why.
	causeRequest startCause = iota

	// causePolicy is the restart policy, and causeRevival the service
	// starting again a run that was running when it went away. Nobody is
	// waiting to be told why either failed, so the run ends exited with the
	// reason as its error.
	causePolicy
	causeRevival
)

// stopCause is why a run's main process is being stopped.
type stopCause int

const (
	// stopByRequest is a client stopping, killing, restarting or deleting
	// the run. The restart policy leaves it be.
	stopByRequest stopCause = iota

	// stopForShutdown is the service shutting down. The run is started
	// again when the service comes back, if its restart policy says so.
	stopForShutdown
)

// isUp is whether a run is, or is about to be, running.
func isUp(state api.State) bool {
	switch state {
	case api.StateStarting, api.StateRunning, api.StateStopping, api.StateRestarting:
		return true
	default:
		return false
	}
}

// Start boots a run's VM and starts its main process. A run that is already
// up is left as it is, as docker leaves a running container.
//
// The image is made sure of first, outside the run's lock, because pulling one
// can take far longer than anything else a run is asked to do: the
// orchestrator pulls before it creates, so this is only ever a long wait for
// an image that went away in between.
func (s *Supervisor) Start(ctx context.Context, id string) (api.Run, error) {
	r, err := s.find(id, true)
	if err != nil {
		return api.Run{}, err
	}

	s.mu.Lock()
	if isUp(r.record.State) {
		view := r.record.Run()
		s.mu.Unlock()

		return view, nil
	}
	reference := r.record.Spec.Image
	s.mu.Unlock()

	image, err := s.image(reference)
	if err != nil {
		return api.Run{}, err
	}

	unlock, err := s.lock(r)
	if err != nil {
		return api.Run{}, err
	}
	defer unlock()

	return s.start(r, image, causeRequest)
}

// Stop stops a run's main process with its image's stop signal, and with
// SIGKILL once timeout is up, and stops its VM. Zero is the default grace. A
// run that is not up is left as it is; one waiting out its restart policy's
// backoff stays exited.
func (s *Supervisor) Stop(ctx context.Context, id string, timeout time.Duration) (api.Run, error) {
	if timeout <= 0 {
		timeout = s.config.StopGrace
	}

	return s.halting(id, func(r *run) error {
		return s.stop(r, timeout, false, stopByRequest)
	})
}

// Kill ends a run's main process with SIGKILL at once, and stops its VM.
func (s *Supervisor) Kill(ctx context.Context, id string) (api.Run, error) {
	return s.halting(id, func(r *run) error {
		return s.stop(r, 0, true, stopByRequest)
	})
}

// Restart stops a run as Stop does and starts it as Start does. It is not one
// of the restart policy's restarts, so the run's restart count stays as it
// was, as docker's does.
func (s *Supervisor) Restart(ctx context.Context, id string, timeout time.Duration) (api.Run, error) {
	if timeout <= 0 {
		timeout = s.config.StopGrace
	}

	r, err := s.find(id, true)
	if err != nil {
		return api.Run{}, err
	}

	// the image first, so that a restart whose image has gone stops
	// nothing.
	s.mu.Lock()
	reference := r.record.Spec.Image
	s.mu.Unlock()

	image, err := s.image(reference)
	if err != nil {
		return api.Run{}, err
	}

	unlock, err := s.lock(r)
	if err != nil {
		return api.Run{}, err
	}
	defer unlock()

	if err := s.stop(r, timeout, false, stopByRequest); err != nil {
		return api.Run{}, err
	}

	return s.start(r, image, causeRequest)
}

// Delete kills a run if it is up, destroys its sandbox, and forgets it, its
// journal included. Its host ports are not handed out again for a while, in
// case a listener of the sandbox outlives it.
func (s *Supervisor) Delete(ctx context.Context, id string) error {
	r, err := s.find(id, false)
	if err != nil {
		return err
	}

	s.hurry(r)

	unlock, err := s.lock(r)
	if err != nil {
		return err
	}
	defer unlock()

	if err := s.stop(r, 0, true, stopByRequest); err != nil {
		return err
	}

	callCtx, cancel := context.WithTimeout(context.Background(), s.config.CallTimeout)
	defer cancel()

	if err := s.sandboxes.Remove(callCtx, SandboxName(id)); err != nil {
		return fmt.Errorf("the run's sandbox could not be destroyed: %w", err)
	}

	s.mu.Lock()
	r.record.Sandbox = false
	s.mu.Unlock()

	// the record goes before anything else does: a run whose record is
	// still there after a restart would come back.
	if err := s.forget(r); err != nil {
		return err
	}

	if err := s.journal.Delete(id); err != nil {
		s.logger.Warn("a deleted run's journal could not be removed", "run", id, "error", err)
	}

	s.hostPorts.Release(id)

	return nil
}

// halting stops or kills a run.
func (s *Supervisor) halting(id string, halt func(r *run) error) (api.Run, error) {
	r, err := s.find(id, false)
	if err != nil {
		return api.Run{}, err
	}

	s.hurry(r)

	unlock, err := s.lock(r)
	if err != nil {
		return api.Run{}, err
	}
	defer unlock()

	if err := halt(r); err != nil {
		return api.Run{}, err
	}

	return s.view(r), nil
}

// hurry kills a main process a stop is already waiting on, rather than wait
// for the run's lock, which that stop holds for as long as its grace period:
// a kill or a delete is not asked to wait for a stop's patience, and docker's
// does not either. The stop then ends with the kill.
func (s *Supervisor) hurry(r *run) {
	s.mu.Lock()
	l := r.live
	stopping := l != nil && r.record.State == api.StateStopping
	s.mu.Unlock()

	if stopping {
		s.signal(l, sigKill)
	}
}

// start boots a run's VM and starts its main process, with the run's lock
// held.
func (s *Supervisor) start(r *run, image ImageConfig, cause startCause) (api.Run, error) {
	s.mu.Lock()

	switch {
	case r.deleted:
		s.mu.Unlock()

		return api.Run{}, notFound(r.id)
	case cause == causePolicy && r.record.State != api.StateRestarting,
		cause != causePolicy && isUp(r.record.State):
		// something else got to it first.
		view := r.record.Run()
		s.mu.Unlock()

		return view, nil
	}

	prior := r.record.State
	spec := cloneSpec(r.record.Spec)

	if s.closing {
		return s.refuseClosing(r, cause)
	}

	s.mu.Unlock()

	argv, err := resolveArgv(spec.Entrypoint, spec.Command, image)
	if err != nil {
		return s.failedStart(r, cause, prior, nil, err)
	}

	admitted := s.admission(spec.Memory)

	s.mu.Lock()

	// checked again under the lock that marks the run starting: Shutdown
	// stops every run it finds up, so a run is either marked before it looks,
	// and stopped by it, or not started at all.
	if s.closing {
		return s.refuseClosing(r, cause)
	}

	if s.admitted+admitted > s.config.Budget {
		inUse := s.admitted
		s.mu.Unlock()

		return s.failedStart(r, cause, prior, nil, newError(api.CodeCapacity,
			"the node's memory budget cannot take the run: its VM takes %s, and %s of %s is in use",
			bytesOf(admitted), bytesOf(inUse), bytesOf(s.config.Budget)))
	}

	s.admitted += admitted

	if cause != causePolicy {
		r.record.State = api.StateStarting
	}

	r.record.StoppedByRequest = false
	r.record.Resume = false
	s.notify(r)

	s.mu.Unlock()

	s.persist(r)

	l, pending, end, err := s.launch(r, spec, argv, stopSignalOf(image), admitted)
	if err != nil {
		s.mu.Lock()
		s.admitted -= admitted
		s.mu.Unlock()

		return s.failedStart(r, cause, prior, end, err)
	}

	s.mu.Lock()

	r.live = l
	r.record.State = api.StateRunning
	r.record.StartedAt = l.startedAt
	r.record.ExitCode = 0
	r.record.Error = ""
	s.notify(r)

	view := r.record.Run()

	s.background.Add(1)
	s.mu.Unlock()

	go s.keep(r, l, pending)

	s.persist(r)

	return view, nil
}

// refuseClosing refuses to start a run because the service is going away,
// with mu held, which it releases.
//
// A run the restart policy or the service's coming back would have started is
// left to be started when the service comes back again, as Shutdown leaves the
// runs it stops, rather than recorded as having failed to start.
func (s *Supervisor) refuseClosing(r *run, cause startCause) (api.Run, error) {
	if cause == causePolicy {
		r.record.State = api.StateExited
		r.record.Error = ReasonServiceRestarted
		r.record.Resume = true
		s.notify(r)
	}

	s.mu.Unlock()

	if cause == causePolicy {
		s.persist(r)
	}

	return api.Run{}, newError(api.CodeUnavailable, "the service is shutting down")
}

// failedStart records a start that failed, and hands back why.
//
// A main process that could not be started ended the run, as a container whose
// command is not there ends with 127. A start that failed before anything ran
// leaves a run a client asked for as it was. A start nobody is waiting on —
// the restart policy's, or a revival's — ends the run, with why as its error,
// since there is nobody else to tell.
func (s *Supervisor) failedStart(r *run, cause startCause, prior api.State, end *ending, err error) (api.Run, error) {
	s.mu.Lock()

	switch {
	case end != nil:
		r.record.State = api.StateExited
		r.record.ExitCode = end.code
		r.record.Error = end.reason
		r.record.FinishedAt = now()
	case cause == causeRequest:
		r.record.State = prior
	default:
		r.record.State = api.StateExited
		r.record.Error = err.Error()
	}

	s.notify(r)
	s.mu.Unlock()

	s.persist(r)

	if cause != causeRequest {
		s.logger.Warn("a run could not be started again", "run", r.id, "error", err)
	}

	return api.Run{}, err
}

// launch boots a run's VM and starts its main process in it, waiting for one
// of the boot slots first.
//
// It is the supervisor's own deadline that bounds it, never a client's: a
// client that went away has not cancelled anything, and cancelling microsandbox
// halfway through making a sandbox leaves a stopped one behind.
//
// A main process that could not be started comes back as how it ended, with
// the VM already stopped. Anything else that went wrong comes back as an error
// alone, nothing having run.
func (s *Supervisor) launch(r *run, spec api.RunSpec, argv []string, stopSignal syscall.Signal, admitted uint64) (*live, []Event, *ending, error) {
	queued := time.NewTimer(s.config.QueueTimeout)
	defer queued.Stop()

	select {
	case s.boots <- struct{}{}:
		defer func() { <-s.boots }()
	case <-queued.C:
		return nil, nil, nil, fmt.Errorf("no VM could be booted within %s: as many as may boot at once were booting", s.config.QueueTimeout)
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.config.BootTimeout)
	defer cancel()

	sandbox, err := s.boot(ctx, r, spec)
	if err != nil {
		return nil, nil, nil, err
	}

	started, err := sandbox.Exec(ctx, Command{Argv: argv})
	if err != nil {
		s.halt(r.id, sandbox, nil)

		return nil, nil, nil, fmt.Errorf("the run's main process could not be started: %w", err)
	}

	// closing a process's handle ends the process, so the handle is closed
	// once, and only after the process has ended or to end it.
	process := &onceClosed{Process: started}

	pending, last, outcome := awaitStart(ctx, process)

	switch outcome {
	case outcomeStarted:
		return &live{
			sandbox:    sandbox,
			process:    process,
			stopSignal: stopSignal,
			admitted:   admitted,
			startedAt:  now(),
			ended:      make(chan struct{}),
		}, pending, nil, nil
	case outcomeFailed:
		s.halt(r.id, sandbox, process)

		end := endingOf(last, true, 0, false)

		return nil, nil, &end, fmt.Errorf("the run's main process %q could not be started: %s", argv[0], end.reason)
	case outcomeLost:
		s.halt(r.id, sandbox, process)

		end := ending{code: exitKilled, reason: ReasonVMLost}

		return nil, nil, &end, errors.New("the run's VM went away before its main process started")
	default:
		s.halt(r.id, sandbox, process)

		return nil, nil, nil, fmt.Errorf("the run's main process did not start within %s", s.config.BootTimeout)
	}
}

// boot boots a run's VM: makes its sandbox at the run's first start, and
// starts the one it already has after that.
func (s *Supervisor) boot(ctx context.Context, r *run, spec api.RunSpec) (Sandbox, error) {
	name := SandboxName(r.id)

	s.mu.Lock()
	made := r.record.Sandbox
	s.mu.Unlock()

	if made {
		sandbox, err := s.sandboxes.Start(ctx, name)
		if err == nil {
			return sandbox, nil
		}

		// a sandbox that is not there any more is made again, on the same
		// host ports. One that is there and will not boot is a failure.
		there, listErr := s.exists(ctx, r.id)
		if listErr != nil || there {
			return nil, fmt.Errorf("the run's VM could not be booted: %w", err)
		}

		s.mu.Lock()
		r.record.Sandbox = false
		s.mu.Unlock()
	}

	endpoints, err := s.endpointsOf(r, spec)
	if err != nil {
		return nil, err
	}

	sandbox, err := s.sandboxes.Create(ctx, s.sandboxSpec(r.id, spec, endpoints))
	if err != nil {
		// microsandbox can leave a stopped sandbox behind a create that
		// failed, which would then refuse the next create by its name. So
		// whatever is left goes, and the next start makes it from scratch.
		removeCtx, cancel := context.WithTimeout(context.Background(), s.config.CallTimeout)
		defer cancel()

		if removeErr := s.sandboxes.Remove(removeCtx, name); removeErr != nil {
			s.logger.Warn("what a failed create left behind could not be destroyed", "run", r.id, "error", removeErr)
		}

		return nil, fmt.Errorf("the run's VM could not be made: %w", err)
	}

	s.mu.Lock()
	r.record.Sandbox = true
	s.mu.Unlock()

	s.persist(r)

	return sandbox, nil
}

// exists is whether a run's sandbox is there.
func (s *Supervisor) exists(ctx context.Context, id string) (bool, error) {
	infos, err := s.sandboxes.List(ctx, map[string]string{LabelRun: id})
	if err != nil {
		return false, err
	}

	for _, info := range infos {
		if info.Name == SandboxName(id) {
			return true, nil
		}
	}

	return false, nil
}

// endpointsOf is where a run's published ports are reached: the host ports it
// was given at its first boot, or new ones if this is its first boot.
func (s *Supervisor) endpointsOf(r *run, spec api.RunSpec) ([]api.Endpoint, error) {
	s.mu.Lock()
	endpoints := slices.Clone(r.record.HostPorts)
	s.mu.Unlock()

	if len(endpoints) == len(spec.Ports) {
		return endpoints, nil
	}

	if len(endpoints) > 0 {
		s.hostPorts.Release(r.id)
	}

	ports, err := s.hostPorts.Allocate(r.id, len(spec.Ports))
	if err != nil {
		return nil, newError(api.CodeCapacity, "the run's ports cannot be published: %v", err)
	}

	endpoints = make([]api.Endpoint, len(spec.Ports))
	for i, port := range spec.Ports {
		endpoints[i] = api.Endpoint{Port: port, HostPort: ports[i]}
	}

	s.mu.Lock()
	r.record.HostPorts = endpoints
	s.mu.Unlock()

	s.persist(r)

	return slices.Clone(endpoints), nil
}

// sandboxSpec is what a run's sandbox is made with.
func (s *Supervisor) sandboxSpec(id string, spec api.RunSpec, endpoints []api.Endpoint) SandboxSpec {
	ports := make([]PortBinding, 0, len(endpoints))
	for _, endpoint := range endpoints {
		ports = append(ports, PortBinding{
			Bind:      s.config.BindAddress,
			HostPort:  endpoint.HostPort,
			GuestPort: endpoint.Port,
		})
	}

	// an isolated guest has no DNS at all; a public one asks the public
	// nameservers rather than docker's, which would tell it the platform's
	// own service names.
	var nameservers []string
	if spec.Network == api.NetworkPublic {
		nameservers = slices.Clone(s.config.Nameservers)
	}

	return SandboxSpec{
		Name:   SandboxName(id),
		Image:  spec.Image,
		CPUs:   spec.CPU,
		Memory: max(spec.Memory, s.config.MemoryFloor),

		// the limit as the task asked for it: the adapter works out the
		// managed disk that leaves that much free beside its journal.
		Disk: spec.Disk,

		Env:         environmentMap(spec.Environment),
		Labels:      labels(id, spec.Node),
		Workdir:     spec.WorkingDir,
		Network:     string(spec.Network),
		Ports:       ports,
		Nameservers: nameservers,

		// what the guest dials. What dials the guest is capped by
		// microsandbox itself, at about 256 new connections to a sandbox
		// every ten seconds; see DefaultMaxTCPConnections.
		MaxTCPConnections: s.config.MaxTCPConnections,
	}
}

// admission is the memory a run's VM is admitted for: its memory, raised to
// the floor, and what microsandbox needs beside it. Microsandbox allocates a
// guest's memory as the guest touches it, so counting every VM at its limit is
// conservative, which is what keeps the host out of its OOM killer.
func (s *Supervisor) admission(memory uint64) uint64 {
	return max(memory, s.config.MemoryFloor) + s.config.Overhead
}

// stop ends a run's main process, with the run's lock held, and returns once
// the run has been recorded. A run that is not up is left as it is, apart
// from a pending restart, which is called off.
func (s *Supervisor) stop(r *run, grace time.Duration, kill bool, cause stopCause) error {
	s.mu.Lock()

	if r.deleted {
		s.mu.Unlock()

		return notFound(r.id)
	}

	if cause == stopByRequest {
		r.record.StoppedByRequest = true
		r.record.Resume = false
	}

	l := r.live

	if l == nil {
		if s.cancelRestart(r) {
			r.record.State = api.StateExited
		}

		s.notify(r)
		s.mu.Unlock()

		s.persist(r)

		return nil
	}

	l.shutdown = cause == stopForShutdown
	r.record.State = api.StateStopping
	s.notify(r)

	s.mu.Unlock()

	s.persist(r)

	s.end(r, l, grace, kill)

	return nil
}

// end ends a live main process and waits for its keeper to have recorded it:
// the stop signal, SIGKILL once grace is up, and the VM stopped under it if
// even that went unanswered.
func (s *Supervisor) end(r *run, l *live, grace time.Duration, kill bool) {
	if !kill {
		s.signal(l, l.stopSignal)

		if waitFor(l.ended, grace) {
			return
		}
	}

	s.signal(l, sigKill)

	if waitFor(l.ended, s.config.KillGrace) {
		return
	}

	// the guest's agent did not answer, so the VM is stopped under the main
	// process, which ends its stream.
	s.logger.Warn("a run's main process outlived SIGKILL, and its VM is being stopped under it", "run", r.id)

	s.mu.Lock()
	l.forced = true
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), s.config.VMStopTimeout+s.config.CallTimeout)
	defer cancel()

	if err := s.sandboxes.Stop(ctx, SandboxName(r.id), s.config.VMStopTimeout); err != nil {
		s.logger.Warn("a run's VM could not be stopped", "run", r.id, "error", err)
	}

	if waitFor(l.ended, s.config.KillGrace) {
		return
	}

	// closing the process's handle ends the process and its stream,
	// whatever state the guest is in.
	_ = l.process.Close()

	if !waitFor(l.ended, s.config.CallTimeout) {
		s.logger.Error("a run's main process could not be ended", "run", r.id)
	}
}

// signal sends a signal to a run's main process, after noting it, so that the
// keeper reading the process's end knows what ended it.
//
// Once SIGKILL has been sent it stays noted: a stop signal sent after it, by a
// stop that was already under way when a kill overtook it, cannot be what
// ended the process.
func (s *Supervisor) signal(l *live, signal syscall.Signal) {
	s.mu.Lock()
	if l.sent != sigKill {
		l.sent = signal
	}
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), s.config.CallTimeout)
	defer cancel()

	// a process that has already gone has nothing to signal, which is what
	// was being asked for anyway.
	_ = l.process.Signal(ctx, signal)
}

// halt stops a run's VM and lets go of its handles.
//
// The VM is stopped first. Closing the sandbox's handle would leave it
// running, sessions and all, and closing a process's handle while the process
// ran would end it, which is what stopping the VM does anyway: by the time the
// handles are closed, there is nothing left for closing them to end.
func (s *Supervisor) halt(id string, sandbox Sandbox, process Process) {
	ctx, cancel := context.WithTimeout(context.Background(), s.config.VMStopTimeout+s.config.CallTimeout)
	defer cancel()

	if err := s.sandboxes.Stop(ctx, SandboxName(id), s.config.VMStopTimeout); err != nil {
		s.logger.Warn("a run's VM could not be stopped", "run", id, "error", err)
	}

	if process != nil {
		_ = process.Close()
	}

	if sandbox != nil {
		_ = sandbox.Close()
	}
}

// cancelRestart calls off a restart the policy is waiting to make, and
// reports whether there was one. Called with mu held.
func (s *Supervisor) cancelRestart(r *run) bool {
	pending := r.pending
	if pending == nil {
		return false
	}

	r.pending = nil
	pending.cancelled = true

	// a timer stopped before it fired never runs what it would have, so what
	// it would have done on its way out is done here.
	if pending.timer.Stop() {
		s.background.Done()
	}

	return true
}

// forget removes a run's record and the run, with the run's lock held.
func (s *Supervisor) forget(r *run) error {
	// no save of the run may land after its record has gone, or it would
	// write the record back.
	r.saving.Lock()
	defer r.saving.Unlock()

	if err := s.records.Delete(r.id); err != nil {
		return fmt.Errorf("the run's record could not be removed: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	r.deleted = true
	delete(s.runs, r.id)

	key := nameKey{node: r.record.Spec.Node, name: r.record.Spec.Name}
	if s.names[key] == r.id {
		delete(s.names, key)
	}

	s.notify(r)

	return nil
}

// waitFor is whether done closes within timeout.
func waitFor(done <-chan struct{}, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

// onceClosed is a process whose handle is closed at most once, however many
// of the paths that end a run get to close it.
type onceClosed struct {
	Process

	once sync.Once
	err  error
}

func (p *onceClosed) Close() error {
	p.once.Do(func() {
		p.err = p.Process.Close()
	})

	return p.err
}

// startOutcome is how waiting for a process to start ended.
type startOutcome int

const (
	outcomeStarted startOutcome = iota
	outcomeFailed
	outcomeLost
	outcomeTimeout
)

// awaitStart reads a process's events until it has started, or until it is
// clear that it will not. What it read past the start, if anything, comes back
// with it, for whoever reads the rest.
func awaitStart(ctx context.Context, process Process) ([]Event, Event, startOutcome) {
	for {
		select {
		case event, open := <-process.Events():
			if !open {
				return nil, Event{Kind: EventLost}, outcomeLost
			}

			switch event.Kind {
			case EventStarted:
				return nil, event, outcomeStarted
			case EventStdout, EventStderr, EventExited:
				// it is running, or has run: the start went unannounced.
				return []Event{event}, event, outcomeStarted
			case EventFailed:
				return nil, event, outcomeFailed
			default:
				return nil, event, outcomeLost
			}
		case <-ctx.Done():
			return nil, Event{}, outcomeTimeout
		}
	}
}

// bytesOf is a size for people.
func bytesOf(size uint64) string {
	const (
		mebibyte = 1 << 20
		gibibyte = 1 << 30
	)

	switch {
	case size >= gibibyte:
		return fmt.Sprintf("%.1f GiB", float64(size)/gibibyte)
	case size >= mebibyte:
		return fmt.Sprintf("%d MiB", size/mebibyte)
	default:
		return fmt.Sprintf("%d bytes", size)
	}
}
