package runs

import (
	"bytes"
	"context"
	"time"
	"unicode/utf8"

	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

// keep is a live run's keeper. It reads the main process's output into the
// run's journal until the process ends, and then does what follows the end of
// a main process: it stops the VM, gives back the memory the VM was admitted
// for, records how the process ended, and applies the run's restart policy.
//
// Nothing else ever reads a main process's events, and nothing else records
// its end, so a stop or a kill only signals the process and waits for this to
// have finished.
func (s *Supervisor) keep(r *run, l *live, pending []Event) {
	defer s.background.Done()

	s.recallLastLine(r)

	journal := &lineWriter{supervisor: s, run: r}

	var (
		last  Event
		ended bool
	)

	consume := func(event Event) bool {
		switch event.Kind {
		case EventStdout:
			journal.write(api.StreamStdout, event.Data)
		case EventStderr:
			journal.write(api.StreamStderr, event.Data)
		case EventExited, EventFailed, EventLost:
			last, ended = event, true
		}

		return ended
	}

	for _, event := range pending {
		if consume(event) {
			break
		}
	}

	if !ended {
		for event := range l.process.Events() {
			if consume(event) {
				break
			}
		}
	}

	journal.flush()

	s.mu.Lock()
	end := endingOf(last, ended, l.sent, l.forced)
	s.mu.Unlock()

	// the VM stops with its main process, so nothing of this run survives
	// into the next but its disk.
	s.halt(r, l.sandbox, l.process)

	s.mu.Lock()

	s.admitted -= l.admitted
	r.live = nil

	// every exec started inside the run went with its VM.
	clear(r.execs)

	finished := now()
	r.record.ExitCode = end.code
	r.record.Error = end.reason
	r.record.FinishedAt = finished

	if l.shutdown {
		r.record.Error = ReasonServiceRestarted
		r.record.Resume = true
	}

	policy, _ := parsePolicy(r.record.Spec.RestartPolicy)

	restart := !l.shutdown &&
		!s.closing &&
		!r.deleted &&
		!r.record.StoppedByRequest &&
		policy.restarts(end.code, r.record.RestartCount)

	if restart {
		r.backoff = s.config.Backoff.next(r.backoff, finished.Sub(l.startedAt))
		r.record.State = api.StateRestarting
		s.scheduleRestart(r, r.backoff)
	} else {
		r.record.State = api.StateExited
	}

	s.notify(r)
	s.mu.Unlock()

	s.persist(r)

	close(l.ended)
}

// scheduleRestart has the restart policy start a run again once wait is up.
// Called with mu held.
func (s *Supervisor) scheduleRestart(r *run, wait time.Duration) {
	pending := &pendingRestart{}
	r.pending = pending

	s.background.Add(1)

	pending.timer = time.AfterFunc(wait, func() {
		defer s.background.Done()

		s.restartByPolicy(r, pending)
	})
}

// restartByPolicy is the restart policy starting a run again, unless the
// restart has been called off in the meantime.
func (s *Supervisor) restartByPolicy(r *run, pending *pendingRestart) {
	s.mu.Lock()
	called := pending.cancelled || r.pending != pending || r.deleted
	reference := r.record.Spec.Image
	s.mu.Unlock()

	if called {
		return
	}

	image, imageErr := s.image(reference)

	unlock, err := s.lock(r)
	if err != nil {
		s.logger.Error("the restart policy could not restart a run", "run", r.id, "error", err)

		return
	}
	defer unlock()

	s.mu.Lock()

	if pending.cancelled || r.pending != pending || r.deleted {
		s.mu.Unlock()

		return
	}

	r.pending = nil
	r.record.RestartCount++

	s.mu.Unlock()

	if imageErr != nil {
		_, _ = s.failedStart(r, causePolicy, api.StateRestarting, nil, imageErr)

		return
	}

	_, _ = s.start(r, image, causePolicy)
}

// recallLastLine reads the stamp of the newest line in a run's journal the
// first time the service needs it, so that a line written after the service
// restarted still comes after every line written before.
func (s *Supervisor) recallLastLine(r *run) {
	s.mu.Lock()
	known := r.lastKnown
	s.mu.Unlock()

	if known {
		return
	}

	last, err := s.journal.Last(r.id)
	if err != nil {
		s.logger.Warn("the newest line of a run's journal could not be read", "run", r.id, "error", err)
	}

	s.mu.Lock()
	if last.After(r.lastAt) {
		r.lastAt = last
	}
	r.lastKnown = true
	s.mu.Unlock()
}

// lineWriter turns a main process's output into the lines of its run's
// journal.
//
// Output arrives in chunks of whatever size, so each stream keeps what it has
// of a line until the line's newline arrives. A line is at most
// api.MaxLogContent bytes: a longer one is cut into lines of that size, so a
// process that writes without newlines cannot grow what is kept of it without
// bound.
type lineWriter struct {
	supervisor *Supervisor
	run        *run

	stdout []byte
	stderr []byte
}

func (w *lineWriter) write(stream string, data []byte) {
	partial := &w.stdout
	if stream == api.StreamStderr {
		partial = &w.stderr
	}

	buffer := append(*partial, data...)

	var lines [][]byte

	for {
		end := bytes.IndexByte(buffer, '\n')
		if end < 0 {
			break
		}

		lines = append(lines, splitLong(buffer[:end])...)
		buffer = buffer[end+1:]
	}

	for len(buffer) >= api.MaxLogContent {
		cut := cutAt(buffer, api.MaxLogContent)
		lines = append(lines, buffer[:cut])
		buffer = buffer[cut:]
	}

	// what is left is kept, copied, since the lines above still point into
	// the buffer it came from.
	*partial = append([]byte(nil), buffer...)

	w.append(stream, lines)
}

// flush writes what is left of each stream's last line, once the process has
// ended and no newline is coming.
func (w *lineWriter) flush() {
	if len(w.stdout) > 0 {
		w.append(api.StreamStdout, [][]byte{w.stdout})
		w.stdout = nil
	}

	if len(w.stderr) > 0 {
		w.append(api.StreamStderr, [][]byte{w.stderr})
		w.stderr = nil
	}
}

// append stamps lines and adds them to the run's journal.
//
// Each line is stamped with the later of now and a nanosecond after the line
// before, so that stamps are unique and strictly increasing within the run,
// which is what lets a reader resume from a line's stamp and lose nothing.
func (w *lineWriter) append(stream string, contents [][]byte) {
	if len(contents) == 0 {
		return
	}

	s, r := w.supervisor, w.run
	lines := make([]api.LogLine, len(contents))

	s.mu.Lock()
	for i, content := range contents {
		at := now()
		if !at.After(r.lastAt) {
			at = r.lastAt.Add(time.Nanosecond)
		}

		r.lastAt = at
		lines[i] = api.LogLine{Stream: stream, At: at, Content: string(content)}
	}
	s.mu.Unlock()

	if err := s.journal.Append(r.id, lines); err != nil {
		s.logger.Error("a run's output could not be written to its journal", "run", r.id, "error", err)
	}

	s.mu.Lock()
	s.notify(r)
	s.mu.Unlock()
}

// splitLong cuts a line longer than api.MaxLogContent into lines that are not.
func splitLong(line []byte) [][]byte {
	var lines [][]byte

	for len(line) > api.MaxLogContent {
		cut := cutAt(line, api.MaxLogContent)
		lines = append(lines, line[:cut])
		line = line[cut:]
	}

	return append(lines, line)
}

// cutAt is where to cut data so that the first part is at most limit bytes,
// moved back to the start of a character when the cut would split one, so
// that each part of valid UTF-8 is still valid UTF-8.
func cutAt(data []byte, limit int) int {
	if len(data) <= limit {
		return len(data)
	}

	for cut := limit; cut > limit-utf8.UTFMax && cut > 0; cut-- {
		if utf8.RuneStart(data[cut]) {
			return cut
		}
	}

	return limit
}

// stopVM stops a sandbox the service found running without a main process,
// as it would have been had the service not gone away.
func (s *Supervisor) stopVM(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), s.config.VMStopTimeout+s.config.CallTimeout)
	defer cancel()

	if err := s.sandboxes.Stop(ctx, name, s.config.VMStopTimeout); err != nil {
		s.logger.Warn("a sandbox could not be stopped", "sandbox", name, "error", err)
	}
}
