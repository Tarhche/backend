// Package waiters is whoever in this process waits for what came of a
// command: the API, asked to answer only once the node has, with ?wait=.
//
// A command's result is heard by whichever control plane its message is
// delivered to, and there may be several of them. The one that hears it
// wakes whoever waits here, at once; one waiting in another learns of it from
// the resource, which keeps the last command's answer, by looking every so
// often. So a wait ends as soon as the answer is heard, wherever it is heard.
package waiters

import (
	"context"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// Waiters are the waits begun in this process, by the command they wait on.
type Waiters struct {
	lock    sync.Mutex
	waiting map[string]map[*Wait]struct{}
}

func New() *Waiters {
	return &Waiters{waiting: make(map[string]map[*Wait]struct{})}
}

// Expect begins waiting for what comes of the command id. It is begun before
// the command is sent, so that no answer can come before anybody waits for
// it, and ended with Done.
func (w *Waiters) Expect(id string) *Wait {
	wait := &Wait{waiters: w, id: id, answer: make(chan kind.Result, 1)}

	w.lock.Lock()
	defer w.lock.Unlock()

	if w.waiting[id] == nil {
		w.waiting[id] = make(map[*Wait]struct{})
	}

	w.waiting[id][wait] = struct{}{}

	return wait
}

// Answer hands what came of a command to everybody here waiting for it. One
// nobody waits for is nobody's business.
func (w *Waiters) Answer(result kind.Result) {
	w.lock.Lock()
	defer w.lock.Unlock()

	for wait := range w.waiting[result.ID] {
		select {
		case wait.answer <- result:
		default:
			// it has its answer already: a result redelivered.
		}
	}
}

// Len is how many waits are begun and not ended, for a test to look at.
func (w *Waiters) Len() int {
	w.lock.Lock()
	defer w.lock.Unlock()

	count := 0
	for _, waits := range w.waiting {
		count += len(waits)
	}

	return count
}

func (w *Waiters) done(wait *Wait) {
	w.lock.Lock()
	defer w.lock.Unlock()

	delete(w.waiting[wait.id], wait)

	if len(w.waiting[wait.id]) == 0 {
		delete(w.waiting, wait.id)
	}
}

// Check looks for a command's answer where it is written down, and says
// whether it found one.
type Check func(ctx context.Context) (kind.Result, bool)

// Wait is one wait for what came of one command.
type Wait struct {
	waiters *Waiters
	id      string
	answer  chan kind.Result
}

// For waits for the answer for as long as timeout, unless ctx ends first, and
// is the answer and whether there was one in time. Every interval it asks
// check, when there is one, whether the answer was heard elsewhere.
func (w *Wait) For(ctx context.Context, timeout time.Duration, interval time.Duration, check Check) (kind.Result, bool) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var ticks <-chan time.Time
	if check != nil && interval > 0 {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		ticks = ticker.C
	}

	for {
		select {
		case result := <-w.answer:
			return result, true
		case <-ticks:
			if result, found := check(ctx); found {
				return result, true
			}
		case <-ctx.Done():
			// one heard just as the time ran out is still in time.
			select {
			case result := <-w.answer:
				return result, true
			default:
				return kind.Result{}, false
			}
		}
	}
}

// Done ends the wait. Whatever comes of the command after is nobody's here.
func (w *Wait) Done() {
	w.waiters.done(w)
}
