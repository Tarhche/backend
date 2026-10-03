// Package report is how the control plane reports a stack. A stack asked for on its
// own and a stack in a listing are the same thing, so they are reported the
// same way and cannot drift apart.
package report

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

// Stack is a stack and the services in it.
type Stack struct {
	UUID  string `json:"uuid"`
	Name  string `json:"name"`
	Slug  string `json:"slug"`
	State string `json:"state"`

	// ExpectedState is what the stack was asked to be. It differs from the
	// state while a command is still reaching its services.
	ExpectedState string `json:"expected_state,omitempty"`

	// Runtime is the one class every service of the stack runs with. One
	// stored before there were classes ran under sysbox, and says so.
	Runtime string `json:"runtime"`

	NodeName  string    `json:"node_name,omitempty"`
	OwnerUUID string    `json:"owner_uuid"`
	Services  []Service `json:"services"`
	CreatedAt time.Time `json:"created_at"`
}

type Service struct {
	UUID          string `json:"uuid"`
	ServiceName   string `json:"service_name"`
	Slug          string `json:"slug"`
	State         string `json:"state"`
	ExpectedState string `json:"expected_state,omitempty"`

	// Runtime is the class the service runs with, which is the stack's, and
	// NodeName the node holding it, empty until one does: they are said of
	// each service as they are of any task, so a service reads the same
	// wherever it is listed.
	Runtime  string `json:"runtime"`
	NodeName string `json:"node_name,omitempty"`

	Image     string     `json:"image"`
	Endpoints []Endpoint `json:"endpoints"`
	CreatedAt time.Time  `json:"created_at"`
	StartedAt time.Time  `json:"started_at"`

	// Deadline is when a service that may only run for so long will be
	// stopped.
	Deadline time.Time `json:"deadline"`
}

type Endpoint struct {
	TaskPort uint `json:"task_port"`
}

// NewStack presents a stack together with its services, whose states are what
// the stack's own state is read off.
func NewStack(s stack.Stack, services []task.Task) Stack {
	items := make([]Service, len(services))
	for i, service := range services {
		items[i] = Service{
			UUID:          service.UUID,
			ServiceName:   service.ServiceName,
			Slug:          service.Slug,
			State:         service.CurrentState.String(),
			ExpectedState: service.ExpectedState.String(),
			Runtime:       service.Runtime.OrSysbox().String(),
			NodeName:      service.NodeName,
			Image:         service.Image,
			Endpoints:     endpoints(service),
			CreatedAt:     service.CreatedAt,
			StartedAt:     service.StartedAt,
			Deadline:      service.Deadline,
		}
	}

	return Stack{
		UUID:          s.UUID,
		Name:          s.Name,
		Slug:          s.Slug,
		State:         stack.State(services).String(),
		ExpectedState: expectedStateOf(s, services).String(),
		Runtime:       s.Runtime.OrSysbox().String(),
		NodeName:      s.NodeName,
		OwnerUUID:     s.OwnerUUID,
		Services:      items,
		CreatedAt:     s.CreatedAt,
	}
}

// expectedStateOf is what a stack was asked to be. A stack from before stacks
// were asked for anything says nothing about it, and what its services were
// asked to be is the best anybody can do for one of those.
func expectedStateOf(s stack.Stack, services []task.Task) task.State {
	if s.ExpectedState != 0 {
		return s.ExpectedState
	}

	return stack.ExpectedState(services)
}

// endpoints reports which task ports are reachable. The host and host port
// a task sits on are the workload's own business, so they stay inside it —
// a caller reaches a port by the task's hostname, not by its node.
//
// Reachable is what the node holding the task said: a port it reaches by
// dialling it, as it does a microVM's, has no host port and is reachable all
// the same.
func endpoints(t task.Task) []Endpoint {
	items := make([]Endpoint, 0, len(t.Endpoints))
	for _, e := range t.Endpoints {
		if e.TaskPort == 0 {
			continue
		}

		items = append(items, Endpoint{TaskPort: uint(e.TaskPort)})
	}

	return items
}
