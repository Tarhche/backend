package getStack

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/stack"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// Response is a stack and the services in it.
type Response struct {
	UUID      string            `json:"uuid"`
	Name      string            `json:"name"`
	Slug      string            `json:"slug"`
	State     string            `json:"state"`
	NodeName  string            `json:"node_name,omitempty"`
	OwnerUUID string            `json:"owner_uuid"`
	Services  []ServiceResponse `json:"services"`
	CreatedAt time.Time         `json:"created_at"`
}

type ServiceResponse struct {
	UUID        string             `json:"uuid"`
	ServiceName string             `json:"service_name"`
	Slug        string             `json:"slug"`
	State       string             `json:"state"`
	Image       string             `json:"image"`
	Endpoints   []EndpointResponse `json:"endpoints"`
	CreatedAt   time.Time          `json:"created_at"`
	StartedAt   time.Time          `json:"started_at"`
}

type EndpointResponse struct {
	ContainerPort uint `json:"container_port"`
}

func NewResponse(s stack.Stack, services []task.Task) *Response {
	items := make([]ServiceResponse, len(services))
	for i, service := range services {
		items[i] = ServiceResponse{
			UUID:        service.UUID,
			ServiceName: service.ServiceName,
			Slug:        service.Slug,
			State:       service.State.String(),
			Image:       service.Image,
			Endpoints:   endpoints(service),
			CreatedAt:   service.CreatedAt,
			StartedAt:   service.StartedAt,
		}
	}

	return &Response{
		UUID:      s.UUID,
		Name:      s.Name,
		Slug:      s.Slug,
		State:     stack.State(services).String(),
		NodeName:  s.NodeName,
		OwnerUUID: s.OwnerUUID,
		Services:  items,
		CreatedAt: s.CreatedAt,
	}
}

// endpoints reports which container ports are reachable. The host and host port
// a container sits on are the runner's own business, so they stay inside it —
// a caller reaches a port by the container's hostname, not by its node.
func endpoints(t task.Task) []EndpointResponse {
	items := make([]EndpointResponse, 0, len(t.Endpoints))
	for _, e := range t.Endpoints {
		if e.HostPort == 0 {
			continue
		}

		items = append(items, EndpointResponse{ContainerPort: uint(e.ContainerPort)})
	}

	return items
}
