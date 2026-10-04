package sdk

import (
	"context"
	"fmt"
	"io"
	"sync"
	"syscall"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
)

// stream is an exec session as a process needs it. The SDK's exec handle is
// the one there is; it is behind this so that what becomes of a session's
// events is tested without a VM.
//
// The SDK lets signal, resize and kill be called while another goroutine
// waits in recv, and nothing else: close in particular comes only once recv
// has returned for the last time.
type stream interface {
	recv(ctx context.Context) (item, error)
	signal(ctx context.Context, signal int) error
	resize(ctx context.Context, rows, cols uint16) error
	kill(ctx context.Context) error
	close() error
}

// item is one thing a stream reported, in the port's terms.
type item struct {
	event runs.Event
	done  bool // the stream has ended and nothing follows
	skip  bool // something the port has no event for, such as a failed write to stdin
}

const (
	// settleTime is how long the rest of a stream is waited for once the
	// command has ended: output the guest had not sent yet, and the stream's
	// end. The guest sends both at once, so this only bounds a stream that
	// never ends.
	settleTime = 5 * time.Second

	// killTime bounds the wait for a command that Close killed to be
	// reported dead, and resizeTime the first resize of a terminal.
	killTime   = 5 * time.Second
	resizeTime = 5 * time.Second

	// eventBuffer is how far the events can run ahead of whoever reads them.
	eventBuffer = 64
)

// process is a command running in a guest: a goroutine reads its stream and
// hands each event on, holding the exit or the failure back until the stream
// has ended, so that it comes last as the port promises.
type process struct {
	stream stream
	stdin  io.WriteCloser
	events chan runs.Event

	// ended is closed once the stream has reported the command's end, and
	// done once the reading goroutine has returned.
	ended     chan struct{}
	endedOnce sync.Once
	done      chan struct{}

	// ctx is cancelled by Close, which ends any wait for the stream.
	ctx    context.Context
	cancel context.CancelFunc

	settle, killWait time.Duration

	closeOnce sync.Once
	closeErr  error
}

var _ runs.Process = &process{}

// newProcess starts reading s. A terminal is sized as soon as the command has
// started, since the SDK cannot start one at a size: until then it is 24 by
// 80.
func newProcess(s stream, stdin io.WriteCloser, command runs.Command) *process {
	return startProcess(s, stdin, command, settleTime, killTime)
}

func startProcess(s stream, stdin io.WriteCloser, command runs.Command, settle, killWait time.Duration) *process {
	ctx, cancel := context.WithCancel(context.Background())
	p := &process{
		stream:   s,
		stdin:    stdin,
		events:   make(chan runs.Event, eventBuffer),
		ended:    make(chan struct{}),
		done:     make(chan struct{}),
		ctx:      ctx,
		cancel:   cancel,
		settle:   settle,
		killWait: killWait,
	}

	var rows, cols uint16
	if command.TTY {
		rows, cols = command.Rows, command.Cols
	}

	go p.read(rows, cols)

	return p
}

func (p *process) Events() <-chan runs.Event {
	return p.events
}

func (p *process) Stdin() io.WriteCloser {
	return p.stdin
}

func (p *process) Signal(ctx context.Context, signal syscall.Signal) error {
	if err := p.stream.signal(ctx, int(signal)); err != nil {
		return fmt.Errorf("microsandbox: signal %d: %w", int(signal), err)
	}

	return nil
}

func (p *process) Resize(ctx context.Context, rows, cols uint16) error {
	if err := p.stream.resize(ctx, rows, cols); err != nil {
		return fmt.Errorf("microsandbox: resize to %dx%d: %w", rows, cols, err)
	}

	return nil
}

// Close ends the command if it is still running, and then lets go of the
// handle. Microsandbox would end it anyway, since it kills a session whose
// handle goes. When the command ends in time, its exit is the last event, as
// it would have been.
func (p *process) Close() error {
	p.closeOnce.Do(func() {
		select {
		case <-p.ended:
		default:
			ctx, cancel := context.WithTimeout(context.Background(), p.killWait)
			_ = p.stream.kill(ctx) // it may have ended meanwhile
			cancel()

			select {
			case <-p.done:
			case <-time.After(p.killWait):
			}
		}

		p.cancel()
		<-p.done

		if err := p.stream.close(); err != nil {
			p.closeErr = fmt.Errorf("microsandbox: close an exec session: %w", err)
		}
	})

	return p.closeErr
}

// read hands each of the stream's events on until it ends, and closes the
// events after the last one.
func (p *process) read(rows, cols uint16) {
	defer close(p.done)
	defer close(p.events)

	var last *runs.Event
	for {
		ctx, cancel := p.ctx, context.CancelFunc(func() {})
		if last != nil {
			ctx, cancel = context.WithTimeout(p.ctx, p.settle)
		}

		it, err := p.stream.recv(ctx)
		cancel()

		switch {
		case err != nil && last != nil:
			// the command ended, but its stream did not end after it
			p.finish(*last)
			return
		case err != nil:
			p.finish(runs.Event{Kind: runs.EventLost, Message: err.Error()})
			return
		case it.done:
			if last == nil {
				last = &runs.Event{Kind: runs.EventLost, Message: "its stream ended without an exit"}
			}
			p.finish(*last)
			return
		case it.skip:
		case it.event.Kind == runs.EventExited, it.event.Kind == runs.EventFailed:
			if last == nil {
				event := it.event
				last = &event
				p.endedOnce.Do(func() { close(p.ended) })
			}
		default:
			if it.event.Kind == runs.EventStarted && rows > 0 && cols > 0 {
				ctx, cancel := context.WithTimeout(p.ctx, resizeTime)
				_ = p.stream.resize(ctx, rows, cols) // a command that is gone has no terminal left to size
				cancel()
			}

			if !p.send(it.event) {
				return
			}
		}
	}
}

// finish hands on the event that ends the stream.
func (p *process) finish(event runs.Event) {
	p.endedOnce.Do(func() { close(p.ended) })

	if !p.send(event) {
		// closed: hand it on only if there is room
		select {
		case p.events <- event:
		default:
		}
	}
}

// send hands an event on, unless the process has been closed meanwhile.
func (p *process) send(event runs.Event) bool {
	select {
	case p.events <- event:
		return true
	case <-p.ctx.Done():
		return false
	}
}
