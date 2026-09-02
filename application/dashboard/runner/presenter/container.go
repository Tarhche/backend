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
	UUID        string     `json:"uuid"`
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	State       string     `json:"state"`
	Image       string     `json:"image"`
	StackUUID   string     `json:"stack_uuid,omitempty"`
	ServiceName string     `json:"service_name,omitempty"`
	Endpoints   []Endpoint `json:"endpoints"`
	Environment []string   `json:"environment,omitempty"`
	Command     []string   `json:"command,omitempty"`
	Entrypoint  []string   `json:"entrypoint,omitempty"`
	WorkingDir  string     `json:"working_dir,omitempty"`
	Limits      Limits     `json:"resource_limits"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  time.Time  `json:"finished_at"`
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
func NewContainer(t task.Task, ingressDomain string) Container {
	return Container{
		UUID:        t.UUID,
		Name:        t.Name,
		Slug:        t.Slug,
		State:       t.State.String(),
		Image:       t.Image,
		StackUUID:   t.StackUUID,
		ServiceName: t.ServiceName,
		Endpoints:   NewEndpoints(t, ingressDomain),
		Environment: t.Environment,
		Command:     t.Command,
		Entrypoint:  t.Entrypoint,
		WorkingDir:  t.WorkingDir,
		Limits: Limits{
			Cpu:    t.ResourceLimits.Cpu,
			Memory: t.ResourceLimits.Memory,
			Disk:   t.ResourceLimits.Disk,
		},
		CreatedAt:  t.CreatedAt,
		StartedAt:  t.StartedAt,
		FinishedAt: t.FinishedAt,
	}
}

// NewContainers presents a list of containers.
func NewContainers(tasks []task.Task, ingressDomain string) []Container {
	items := make([]Container, len(tasks))
	for i := range tasks {
		items[i] = NewContainer(tasks[i], ingressDomain)
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
		if e.HostPort == 0 || len(t.Slug) == 0 {
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
