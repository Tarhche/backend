// Package lock keeps what a node does to one resource from overlapping.
//
// A node is asked to do things to what it holds by messages, several of them
// at once, and the same thing more than once: a command is delivered again
// when its first delivery was not acknowledged in time, and the control plane
// asks again for what it has not seen happen. Two creates of one VM racing
// would leave the second refused and reported as a failure while the VM
// runs, and a start racing a restore would boot the disk that is being
// replaced. So whatever changes a resource, every command of every kind and a
// snapshot taken of a VM, holds its lock while it does, and the next one
// waits its turn. Different resources never wait for each other.
package lock

import (
	"context"
	"sync"
)

// Locks is one lock per VM, made when it is first held and let go when it is
// released.
type Locks struct {
	mu   sync.Mutex
	held map[string]chan struct{}
}

// New is a set of locks none of which is held.
func New() *Locks {
	return &Locks{held: make(map[string]chan struct{})}
}

// Lock holds the lock of one VM, waiting for whoever holds it now, and hands
// back what releases it. Giving up on waiting is ctx ending, which holds
// nothing.
func (l *Locks) Lock(ctx context.Context, vmUUID string) (func(), error) {
	for {
		l.mu.Lock()

		released, busy := l.held[vmUUID]
		if !busy {
			released = make(chan struct{})
			l.held[vmUUID] = released
			l.mu.Unlock()

			var once sync.Once

			return func() {
				once.Do(func() {
					l.mu.Lock()
					delete(l.held, vmUUID)
					l.mu.Unlock()

					close(released)
				})
			}, nil
		}

		l.mu.Unlock()

		select {
		case <-released:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}
