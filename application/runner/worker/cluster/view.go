// Package cluster is what one node knows about all of them.
//
// Every node says what it is holding several times a second, and every node
// listens: a beat carries a container's name, its state and the addresses its
// ports came up on, which is everything needed to route a request to it. So a
// node can answer for a container it is not holding by passing the request to
// the node that is, without asking a database or a manager where anything is.
package cluster

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

// staleAfter is how long a container is remembered after its node stopped
// speaking for it. Nodes beat several times a second, so silence this long is
// a node that is gone rather than one that is busy.
const staleAfter = 15 * time.Second

type held struct {
	container task.Task
	node      string
	seen      time.Time
}

// View is what this node knows of the containers the runner is holding.
type View struct {
	// node is this one's name, which is what tells its own containers from
	// everybody else's: a node serves the ports of what it holds.
	node string

	// now is read rather than called directly, so a test can say what time it
	// is.
	now func() time.Time

	mutex sync.RWMutex
	held  map[string]held
}

var _ domain.MessageHandler = &View{}

func NewView(node string) *View {
	return &View{
		node: node,
		now:  time.Now,
		held: make(map[string]held),
	}
}

// Handle takes in what a node said about one of its containers.
func (v *View) Handle(ctx context.Context, data []byte) error {
	var beat events.Heartbeat
	if err := json.Unmarshal(data, &beat); err != nil {
		// a beat nobody can read says nothing about anything; the next one
		// will be along shortly.
		return nil
	}

	if beat.Slug == "" {
		return nil
	}

	state := task.State(beat.State)

	v.mutex.Lock()
	defer v.mutex.Unlock()

	// a container that has ended is not worth routing to, and saying so at
	// once is better than waiting for its node to fall silent.
	if task.IsTerminalState(state) {
		delete(v.held, beat.Slug)

		return nil
	}

	v.held[beat.Slug] = held{
		node: beat.NodeName,
		seen: v.now(),
		container: task.Task{
			UUID:         beat.UUID,
			Name:         beat.Name,
			Slug:         beat.Slug,
			NodeName:     beat.NodeName,
			CurrentState: state,
			Endpoints:    endpointsOf(beat.Endpoints),
		},
	}

	return nil
}

// GetOneBySlug finds the container a hostname names, wherever it is being
// held. It is what the ingress asks of this view.
func (v *View) GetOneBySlug(ctx context.Context, slug string) (task.Task, error) {
	v.mutex.RLock()
	defer v.mutex.RUnlock()

	known, ok := v.held[slug]
	if !ok || v.stale(known) {
		return task.Task{}, domain.ErrNotExists
	}

	return known.container, nil
}

// GetRunningWithPublicPorts is what this node itself is holding and forwarding
// whole. Another node's ports are that node's to listen on: only one of them
// can hold the socket, and it is the one the container is on.
func (v *View) GetRunningWithPublicPorts(ctx context.Context) ([]task.Task, error) {
	v.mutex.RLock()
	defer v.mutex.RUnlock()

	var mine []task.Task

	for _, known := range v.held {
		if known.node != v.node || v.stale(known) {
			continue
		}

		if known.container.CurrentState != task.Running {
			continue
		}

		mine = append(mine, known.container)
	}

	return mine, nil
}

// Forget drops what has not been spoken for lately, so a node that goes away
// takes its containers out of everybody's view with it.
func (v *View) Forget() {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	for slug, known := range v.held {
		if v.stale(known) {
			delete(v.held, slug)
		}
	}
}

func (v *View) stale(known held) bool {
	return v.now().Sub(known.seen) > staleAfter
}

func endpointsOf(endpoints []events.Endpoint) []task.Endpoint {
	result := make([]task.Endpoint, len(endpoints))
	for i, e := range endpoints {
		result[i] = task.Endpoint{
			ContainerPort: e.ContainerPort,
			Host:          e.Host,
			HostPort:      e.HostPort,
			HostPortUDP:   e.HostPortUDP,
			PublicPort:    e.PublicPort,
			PublicHost:    e.PublicHost,
		}
	}

	return result
}
