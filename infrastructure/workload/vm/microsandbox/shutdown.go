//go:build microsandbox

package microsandbox

import (
	"context"
	"maps"
	"slices"
	"sync"
	"time"
)

// Shutdown stops every running VM, all at once, before the vmhost goes.
//
// A VM stopped this way flushes its disk and keeps everything it wrote; one
// that goes down with its container loses what the guest had not written yet.
// A Docker VM's dockerd is stopped first, which stops its containers the way
// docker stops them. A VM that does not stop in time is killed, and those
// still going when ctx ends are given up on. Nothing is booted afterwards,
// and a main process stopped here is one its instance lost with the vmhost,
// which the next vmhost says when it starts.
func (e *engine) Shutdown(ctx context.Context) error {
	e.lock.Lock()
	e.closing = true
	held := slices.Collect(maps.Values(e.instances))
	e.lock.Unlock()

	statuses, err := e.statuses(ctx)
	if err != nil {
		return err
	}

	stopping := time.Now()

	var stopped sync.WaitGroup

	for _, i := range held {
		if !up(statuses[i.id]) {
			continue
		}

		stopped.Go(func() {
			e.halt(i)
			defer e.forget(i)

			h, err := e.sandboxOf(ctx, i.id)
			if err != nil || h == nil {
				return
			}

			if err := e.shutdownSandbox(ctx, i, h, stopTimeout); err != nil {
				e.logger.Warn("a vm could not be stopped as the vmhost went", "vm", i.id, "error", err)
			}
		})
	}

	done := make(chan struct{})
	go func() {
		stopped.Wait()
		close(done)
	}()

	select {
	case <-done:
		e.logger.Info("every vm was stopped", "took", time.Since(stopping))

		return nil
	case <-ctx.Done():
		e.logger.Warn("the vmhost went before every vm had stopped", "took", time.Since(stopping))

		return ctx.Err()
	}
}
