//go:build linux

package agent

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"
)

// The groups the agent keeps what it starts in: the task's own process, and
// each command run beside it. The task as a whole is every one of them, and is
// named by no name at all.
const mainGroup = "main"

func execGroup(id string) string {
	return "exec-" + id
}

// confinement is what the agent keeps what it starts in, so that each thing
// it started can be signalled, waited for and ended with everything it
// started in turn, and the task with all of it.
type confinement interface {
	// prepare readies the group called name for a process to be started in,
	// and hands back the cgroup to start it inside, when there is one.
	prepare(name string) (*os.File, error)

	// started says which process was started in the group called name.
	started(name string, pid int)

	// populated reports whether anything still runs in the group called
	// name.
	populated(name string) (bool, error)

	// signal sends a signal to everything in the group called name, and
	// reports whether there was anything to send it to.
	signal(name string, signal syscall.Signal) (bool, error)

	// kill ends everything in the group called name, or the whole task for
	// no name, and waits for it to be gone.
	kill(name string, timeout time.Duration) error

	// remove kills, and then forgets the group, which is made again the
	// next time a process is started in it.
	remove(name string, timeout time.Duration) error

	// usage is what the task as a whole has used, as far as can be told.
	usage() usage
}

// usage is what the task has used: CPU in microseconds, and the processes it
// has now.
type usage struct {
	cpu  uint64
	pids uint64
}

// processGroups keeps what the task starts in process groups, for a machine
// whose kernel has no cgroups — and for the agent's tests, which run as
// somebody who may make none.
//
// Every process the agent starts is the leader of a session of its own, so
// what it starts is in its process group unless it leaves: a process can
// leave a process group, which it cannot leave a cgroup, so this is the lesser
// of the two. Nothing can join a process group that is gone, so a group found
// empty is forgotten and its number never signalled again.
type processGroups struct {
	lock   sync.Mutex
	groups map[string]int
}

var _ confinement = (*processGroups)(nil)

func newProcessGroups() *processGroups {
	return &processGroups{groups: make(map[string]int)}
}

func (p *processGroups) prepare(string) (*os.File, error) {
	return nil, nil
}

func (p *processGroups) started(name string, pid int) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.groups[name] = pid
}

// leaders is the process groups the group called name is, by their leaders.
func (p *processGroups) leaders(name string) map[string]int {
	p.lock.Lock()
	defer p.lock.Unlock()

	leaders := make(map[string]int)
	for group, leader := range p.groups {
		if len(name) == 0 || group == name {
			leaders[group] = leader
		}
	}

	return leaders
}

func (p *processGroups) populated(name string) (bool, error) {
	for _, leader := range p.leaders(name) {
		if inhabited(leader) {
			return true, nil
		}
	}

	return false, nil
}

func (p *processGroups) signal(name string, signal syscall.Signal) (bool, error) {
	signalled := false
	for _, leader := range p.leaders(name) {
		if err := syscall.Kill(-leader, signal); err == nil {
			signalled = true
		}
	}

	return signalled, nil
}

func (p *processGroups) kill(name string, timeout time.Duration) error {
	leaders := p.leaders(name)
	for _, leader := range leaders {
		_ = syscall.Kill(-leader, syscall.SIGKILL)
	}

	deadline := time.Now().Add(timeout)
	for group, leader := range leaders {
		for inhabited(leader) {
			if time.Now().After(deadline) {
				return fmt.Errorf("the process group of %s still holds processes after %s", group, timeout)
			}

			time.Sleep(emptyInterval)
		}

		p.lock.Lock()
		if p.groups[group] == leader {
			delete(p.groups, group)
		}
		p.lock.Unlock()
	}

	return nil
}

func (p *processGroups) remove(name string, timeout time.Duration) error {
	return p.kill(name, timeout)
}

func (p *processGroups) usage() usage {
	return usage{}
}

// inhabited reports whether anything is left of the process group a process
// leads.
func inhabited(leader int) bool {
	err := syscall.Kill(-leader, 0)

	return err == nil || errors.Is(err, syscall.EPERM)
}
