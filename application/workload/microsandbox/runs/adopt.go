package runs

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

// Open makes the service ready, and returns once it is or once ctx is done.
//
// Nothing that ran under the service before it last went away is still
// running: microsandbox ended every main process with the service that
// started it. What is left is the records of the runs, their journals, and
// their sandboxes, stopped or with nothing running in them. So Open:
//
//  1. checks the runtime: /dev/kvm, the msb and libkrunfw it runs, and that
//     msb's version is the SDK's, since the two share one database;
//  2. reads the records of the runs, and holds their host ports again;
//  3. lists the sandboxes the service made;
//  4. reconciles the two: a run that was up is recorded as ended by the
//     service going away, a VM still running without its main process is
//     stopped, a run whose sandbox has gone gets a new one at its next
//     start, on the same host ports, and a sandbox with no run is destroyed;
//  5. starts again the runs whose restart policy brings them back, as many at
//     once as may boot at once;
//  6. and is ready.
//
// Until step 4 is done, reading runs is unavailable, since a node that seems
// to hold nothing would have its tasks scheduled again; until step 6 is done,
// changing them is. A step that fails is tried again, the wait between tries
// doubling, so a runtime that comes right by itself is picked up without a
// restart.
func (s *Supervisor) Open(ctx context.Context) error {
	wait := s.config.RetryInterval

	for {
		err := s.open(ctx)
		if err == nil {
			return nil
		}

		s.mu.Lock()
		s.reason = err.Error()
		closing := s.closing
		s.mu.Unlock()

		if closing {
			return err
		}

		s.logger.Error("the service is not ready", "error", err, "retry", wait)

		timer := time.NewTimer(wait)

		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()

			return ctx.Err()
		}

		wait = min(2*wait, s.config.RetryMaxInterval)
	}
}

func (s *Supervisor) open(ctx context.Context) error {
	if err := s.check(ctx); err != nil {
		return err
	}

	if err := s.load(); err != nil {
		return err
	}

	revive, err := s.reconcile(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.loaded = true
	s.reason = "the runs that were running are being started again"
	s.mu.Unlock()

	s.revive(revive)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closing {
		return fmt.Errorf("the service is shutting down")
	}

	s.ready = true
	s.reason = ""

	s.logger.Info("the service is ready", "runs", len(s.runs), "revived", len(revive))

	return nil
}

// check checks the runtime, and that msb and the SDK are of one version.
func (s *Supervisor) check(ctx context.Context) error {
	callCtx, cancel := context.WithTimeout(ctx, s.config.CallTimeout)
	defer cancel()

	versions, err := s.sandboxes.Check(callCtx)
	if err != nil {
		return fmt.Errorf("microsandbox cannot run anything here: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.versions = versions

	if sdk, runtime := versionOf(versions.SDK), versionOf(versions.Runtime); len(sdk) == 0 || sdk != runtime {
		s.checked = false

		return fmt.Errorf("msb is %q and the SDK is %q: they share one database, which two versions break", versions.Runtime, versions.SDK)
	}

	s.checked = true

	return nil
}

// versionOf is a version as msb and the SDK both mean it: "msb 0.7.6", "v0.7.6"
// and "0.7.6" are all 0.7.6.
func versionOf(value string) string {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}

	return strings.TrimPrefix(fields[len(fields)-1], "v")
}

// load reads the records of the runs, once.
func (s *Supervisor) load() error {
	s.mu.Lock()
	read := s.recordsRead
	s.mu.Unlock()

	if read {
		return nil
	}

	records, err := s.records.Load()
	if err != nil {
		return fmt.Errorf("the runs' records could not be read: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.recordsRead = true

	for _, record := range records {
		r := &run{
			id:      record.ID,
			op:      make(chan struct{}, 1),
			record:  record,
			execs:   make(map[string]*Exec),
			changed: make(chan struct{}),
		}

		s.runs[record.ID] = r
		s.names[nameKey{node: record.Spec.Node, name: record.Spec.Name}] = record.ID

		ports := make([]uint16, 0, len(record.HostPorts))
		for _, endpoint := range record.HostPorts {
			ports = append(ports, endpoint.HostPort)
		}

		if len(ports) > 0 {
			s.hostPorts.Hold(record.ID, ports)
		}
	}

	return nil
}

// reconcile brings the records into line with the sandboxes that are there,
// and reports the runs the restart policy brings back.
func (s *Supervisor) reconcile(ctx context.Context) ([]*run, error) {
	listCtx, cancel := context.WithTimeout(ctx, s.config.CallTimeout)
	defer cancel()

	infos, err := s.sandboxes.List(listCtx, map[string]string{LabelManaged: "true"})
	if err != nil {
		return nil, fmt.Errorf("the sandboxes could not be listed: %w", err)
	}

	found := make(map[string]SandboxInfo, len(infos))
	var orphans []SandboxInfo

	s.mu.Lock()

	for _, info := range infos {
		id := info.Labels[LabelRun]

		if _, known := s.runs[id]; known && info.Name == SandboxName(id) {
			found[id] = info

			continue
		}

		orphans = append(orphans, info)
	}

	held := make([]*run, 0, len(s.runs))
	for _, r := range s.runs {
		held = append(held, r)
	}

	s.mu.Unlock()

	var revive []*run

	for _, r := range held {
		info, there := found[r.id]

		// its main process went with the service that started it, so its VM
		// is stopped, as it would have been.
		if there && info.Running {
			s.stopVM(info.Name)
		}

		s.mu.Lock()

		up := isUp(r.record.State)

		if up {
			// a run waiting out its backoff had already ended, and keeps its
			// exit code; any other was cut short.
			if r.record.State != api.StateRestarting {
				r.record.ExitCode = exitKilled
				r.record.FinishedAt = now()
			}

			r.record.Error = ReasonServiceRestarted
			r.record.State = api.StateExited
		}

		if !there {
			r.record.Sandbox = false
		}

		policy, _ := parsePolicy(r.record.Spec.RestartPolicy)

		if (up || r.record.Resume) && !r.record.StoppedByRequest && policy.revives() {
			revive = append(revive, r)
		}

		r.record.Resume = false

		s.mu.Unlock()

		s.persist(r)
	}

	for _, orphan := range orphans {
		s.logger.Warn("destroying a sandbox that belongs to no run", "sandbox", orphan.Name)

		removeCtx, cancel := context.WithTimeout(ctx, s.config.CallTimeout)
		if err := s.sandboxes.Remove(removeCtx, orphan.Name); err != nil {
			s.logger.Warn("a sandbox that belongs to no run could not be destroyed", "sandbox", orphan.Name, "error", err)
		}
		cancel()
	}

	return revive, nil
}

// revive starts runs again because the service came back, as many at once as
// may boot at once.
func (s *Supervisor) revive(runs []*run) {
	var wg sync.WaitGroup

	for _, r := range runs {
		wg.Add(1)

		go func() {
			defer wg.Done()

			s.mu.Lock()
			reference := r.record.Spec.Image
			s.mu.Unlock()

			image, imageErr := s.image(reference)

			unlock, err := s.lock(r)
			if err != nil {
				s.logger.Error("a run could not be started again", "run", r.id, "error", err)

				return
			}
			defer unlock()

			if imageErr != nil {
				_, _ = s.failedStart(r, causeRevival, api.StateExited, nil, imageErr)

				return
			}

			_, _ = s.start(r, image, causeRevival)
		}()
	}

	wg.Wait()
}

// Shutdown stops every run that is up, because the service is going away and
// its main processes would go with it anyway: stopped, they end as they would
// have been asked to, with their stop signal and their grace period.
//
// Nothing is started from the moment it begins. A run it stops is recorded as
// ended by the service, not by a request, and its restart policy decides at
// the service's next start whether it is started again; so does that of a run
// that was waiting out its backoff. It returns once every run has been
// recorded, or once ctx is done.
func (s *Supervisor) Shutdown(ctx context.Context) error {
	s.mu.Lock()

	s.closing = true

	var (
		up      []*run
		waiting []*run
	)

	for _, r := range s.runs {
		if s.cancelRestart(r) {
			r.record.State = api.StateExited
			r.record.Error = ReasonServiceRestarted
			r.record.Resume = true
			s.notify(r)

			waiting = append(waiting, r)
		}

		if r.live != nil || r.record.State == api.StateStarting {
			up = append(up, r)
		}
	}

	s.mu.Unlock()

	for _, r := range waiting {
		s.persist(r)
	}

	var wg sync.WaitGroup

	for _, r := range up {
		wg.Add(1)

		go func() {
			defer wg.Done()

			unlock, err := s.lock(r)
			if err != nil {
				s.logger.Error("a run could not be stopped as the service shut down", "run", r.id, "error", err)

				return
			}
			defer unlock()

			_ = s.stop(r, s.config.StopGrace, false, stopForShutdown)
		}()
	}

	done := make(chan struct{})

	go func() {
		wg.Wait()
		s.background.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("not every run was stopped before the service had to go: %w", ctx.Err())
	}
}
