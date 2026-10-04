package runs

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

// Logs hands emit every line of a run's journal from since on, since
// included, as docker's since is. With follow it carries on handing it lines
// for as long as the run is up — starting, running, stopping or waiting out
// its restart policy's backoff — and returns once the run has ended and every
// line it wrote has been handed over, or once ctx is done.
//
// It returns emit's error as soon as emit refuses a line, which is how a
// caller that has had enough stops it.
func (s *Supervisor) Logs(ctx context.Context, id string, since time.Time, follow bool, emit func(api.LogLine) error) error {
	s.mu.Lock()

	if err := s.readable(); err != nil {
		s.mu.Unlock()

		return err
	}

	r, found := s.runs[id]

	s.mu.Unlock()

	if !found {
		return notFound(id)
	}

	reader, err := s.journal.Reader(id, since)
	if err != nil {
		return err
	}
	defer reader.Close()

	for {
		// what wakes this is taken before the journal is read to its end,
		// so a line appended after that read, or the run ending after it,
		// is never missed.
		s.mu.Lock()
		changed := r.changed
		up := !r.deleted && isUp(r.record.State)
		s.mu.Unlock()

		for {
			line, err := reader.Next()
			if errors.Is(err, io.EOF) {
				break
			}

			if err != nil {
				return err
			}

			if err := emit(line); err != nil {
				return err
			}
		}

		if !follow || !up {
			return nil
		}

		select {
		case <-changed:
		case <-ctx.Done():
			return nil
		}
	}
}
