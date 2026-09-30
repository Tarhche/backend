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
	LastHeartbeatAt time.Time `json:"last_heartbeat_at"`
	CreatedAt       time.Time `json:"created_at"`
}

type Pagination struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

func NewResponse(tasks []node.Node, totalPages, currentPage uint) *Response {
	items := make([]NodeResponse, len(tasks))

	for i, t := range tasks {
		items[i] = NodeResponse{
			Name:            t.Name,
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
