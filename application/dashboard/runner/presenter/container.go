// Package presenter turns what the runner reports into what the dashboard
// shows. The shapes live in one place because a container looks the same
// whether it is listed on its own or as a service of a stack.
package presenter

import (
	"fmt"
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// Container is one container, as the dashboard shows it.
type Container struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	Slug string `json:"slug"`
	// State is what the container is doing; ExpectedState is what it was asked
	// to be doing. They differ while the runner is closing the gap.
	State         string     `json:"state"`
	ExpectedState string     `json:"expected_state,omitempty"`
	Image         string     `json:"image"`
	StackUUID     string     `json:"stack_uuid,omitempty"`
	ServiceName   string     `json:"service_name,omitempty"`
	Endpoints     []Endpoint `json:"endpoints"`
	Environment   []string   `json:"environment,omitempty"`
	Command       []string   `json:"command,omitempty"`
	Entrypoint    []string   `json:"entrypoint,omitempty"`
	WorkingDir    string     `json:"working_dir,omitempty"`
	ReadOnly      bool       `json:"read_only"`

	// MaxRetries is how many times a container that fails is asked for again
	// before the runner gives up on it. -1 never gives up. Retries is how many
	// of those have happened, so a container that keeps failing can say what is
	// being done about it.
	MaxRetries int `json:"max_retries"`
	Retries    int `json:"retries"`

	// Reason is why a container failed, when the runner can say so.
	Reason string `json:"reason,omitempty"`

	// Owner is who asked for this container.
	Owner      Owner     `json:"owner"`
	Limits     Limits    `json:"resource_limits"`
	CreatedAt  time.Time `json:"created_at"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`

	// Deadline is when a container that is only allowed to run for so long
	// will be stopped. A container with no limit of its own has none.
	Deadline *time.Time `json:"deadline,omitempty"`
}

// Endpoint is one of a container's exposed ports, together with the hostname it
// answers on. The node and host port behind it stay inside the runner: a caller
// reaches a port by the container's own name.
type Endpoint struct {
	ContainerPort uint   `json:"container_port"`
	Host          string `json:"host"`
	URL           string `json:"url"`
}

type Limits struct {
	Cpu    float64 `json:"cpu"`
	Memory uint64  `json:"memory"`
	Disk   uint64  `json:"disk"`
}

// NewContainer presents one container. The ingress domain is what its hostnames
// are built from, so the dashboard can link straight to a running container.
func NewContainer(t task.Task, ingressDomain string, owners Owners) Container {
	return Container{
		UUID:          t.UUID,
		Name:          t.Name,
		Slug:          t.Slug,
		State:         t.CurrentState.String(),
		ExpectedState: t.ExpectedState.String(),
		Image:         t.Image,
		StackUUID:     t.StackUUID,
		ServiceName:   t.ServiceName,
		Endpoints:     NewEndpoints(t, ingressDomain),
		Environment:   t.Environment,
		Command:       t.Command,
		Entrypoint:    t.Entrypoint,
		WorkingDir:    t.WorkingDir,
		ReadOnly:      t.ReadOnly,
		MaxRetries:    t.MaxRetries,
		Retries:       t.Retries,
		Reason:        t.Reason,
		Owner:         owners.Of(t.OwnerUUID),
		Limits: Limits{
			Cpu:    t.ResourceLimits.Cpu,
			Memory: t.ResourceLimits.Memory,
			Disk:   t.ResourceLimits.Disk,
		},
		CreatedAt:  t.CreatedAt,
		StartedAt:  t.StartedAt,
		FinishedAt: t.FinishedAt,
		Deadline:   deadline(t),
	}
}

// deadline is when a container that may only run for so long will be stopped.
// The node that made it sets it as it comes up, so a container that is not
// running, or that may run forever, has nothing to count down to.
func deadline(t task.Task) *time.Time {
	if t.Deadline.IsZero() || t.CurrentState != task.Running {
		return nil
	}

	at := t.Deadline

	return &at
}

// NewContainers presents a list of containers.
func NewContainers(tasks []task.Task, ingressDomain string, owners Owners) []Container {
	items := make([]Container, len(tasks))
	for i := range tasks {
		items[i] = NewContainer(tasks[i], ingressDomain, owners)
	}

	return items
}

// NewEndpoints builds the addresses a container's ports are served on.
//
// The first exposed port answers on the container's bare name, and every port
// also answers on that name with the port appended, which keeps each hostname
// to a single label so one wildcard certificate covers them all.
func NewEndpoints(t task.Task, ingressDomain string) []Endpoint {
	endpoints := make([]Endpoint, 0, len(t.Endpoints))

	for i, e := range t.Endpoints {
		// the runner reports only the ports it actually published, and keeps
		// the node and host port behind them to itself, so there is nothing
		// here to filter on beyond having a name to serve them under.
		if len(t.Slug) == 0 {
			continue
		}

		host := fmt.Sprintf("%s-%d.%s", t.Slug, e.ContainerPort, ingressDomain)
		if i == 0 {
			host = fmt.Sprintf("%s.%s", t.Slug, ingressDomain)
		}

		endpoints = append(endpoints, Endpoint{
			ContainerPort: uint(e.ContainerPort),
			Host:          host,
			URL:           "http://" + host,
		})
	}

	return endpoints
}
