package vmhost

import (
	"context"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Logs hands what a VM's task wrote to emit, in order: every line numbered
// after after, written no earlier than since (when it is not zero). With
// follow it keeps handing what comes until the VM's task has ended and nobody
// will write any more of it, emit refuses a line, or ctx is done.
//
// Lines are read from vmhost's own copy, never from the guest, so a VM that
// ended — or whose machine went away — is read like one that runs, and a
// reader that resumes after the last number it saw misses nothing and sees
// nothing twice, however many times the VM was started meanwhile.
func (e *Engine) Logs(ctx context.Context, id string, after uint64, since time.Time, follow bool, emit func(vm.LogLine) error) error {
	if _, err := e.states.Get(ctx, id); err != nil {
		return err
	}

	reader := e.logs.Reader(id)

	forward := func(line vm.LogLine) error {
		if line.Seq <= after || (!since.IsZero() && line.At.Before(since)) {
			return nil
		}

		return emit(line)
	}

	for {
		// what is waited on is taken before reading, so a line kept in
		// between is not missed.
		var changed <-chan struct{}
		if k := e.keeper(id); k != nil {
			changed = k.out.Changed()
		}

		if err := reader.Next(forward); err != nil {
			return err
		}

		if !follow {
			return nil
		}

		v, err := e.states.Get(ctx, id)
		if err != nil {
			// deleted while it was being read: there is no more of it.
			return nil
		}

		// a VM that ended, and is not on its way back up, writes no more.
		// What its keeper kept last is read once more first.
		if !v.State.Up() && e.keeper(id) == nil {
			return reader.Next(forward)
		}

		timer := time.NewTimer(e.timing.poll)

		select {
		case <-changed:
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()

			return nil
		}

		timer.Stop()
	}
}
