package getNodes

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/node"
)

type Response struct {
	Items      []NodeResponse `json:"items"`
	Pagination Pagination     `json:"pagination"`
}

type NodeResponse struct {
	Name            string    `json:"name"`
	Resources       Resource  `json:"resources"`
	LastHeartbeatAt time.Time `json:"last_heartbeat_at"`
	CreatedAt       time.Time `json:"created_at"`
}

type Resource struct {
	Cpu    float64 `json:"cpu"`
	Memory uint64  `json:"memory"`
	Disk   uint64  `json:"disk"`
}

type Pagination struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

func NewResponse(tasks []node.Node, totalPages, currentPage uint) *Response {
	items := make([]NodeResponse, len(tasks))

	for i, t := range tasks {
		items[i] = NodeResponse{
			Name: t.Name,
			Resources: Resource{
				Cpu:    t.Resources.Cpu,
				Memory: t.Resources.Memory,
				Disk:   t.Resources.Disk,
			},
			LastHeartbeatAt: t.LastHeartbeatAt,
			CreatedAt:       t.CreatedAt,
		}
	}

	return &Response{
		Items: items,
		Pagination: Pagination{
			TotalPages:  totalPages,
			CurrentPage: currentPage,
		},
	}
}
