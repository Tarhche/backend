package getNode

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/node"
)

type Response struct {
	Name            string    `json:"name"`
	Stats           Stats     `json:"stats"`
	LastHeartbeatAt time.Time `json:"last_heartbeat_at"`
	CreatedAt       time.Time `json:"created_at"`
}

type Stats struct {
	PIDs          uint64  `json:"pids"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryUsage   uint64  `json:"memory_usage"`
	MemoryLimit   uint64  `json:"memory_limit"`
	MemoryPercent float64 `json:"memory_percent"`
	NetworkInput  uint64  `json:"network_input"`
	NetworkOutput uint64  `json:"network_output"`
	BlockInput    uint64  `json:"block_input"`
	BlockOutput   uint64  `json:"block_output"`
}

func NewResponse(node *node.Node) *Response {
	return &Response{
		Name: node.Name,
		Stats: Stats{
			PIDs:          node.Stats.PIDs,
			CPUPercent:    node.Stats.CPUPercent,
			MemoryUsage:   node.Stats.MemoryUsage,
			MemoryLimit:   node.Stats.MemoryLimit,
			MemoryPercent: node.Stats.MemoryPercent,
			NetworkInput:  node.Stats.NetworkInput,
			NetworkOutput: node.Stats.NetworkOutput,
			BlockInput:    node.Stats.BlockInput,
			BlockOutput:   node.Stats.BlockOutput,
		},
		LastHeartbeatAt: node.LastHeartbeatAt,
		CreatedAt:       node.CreatedAt,
	}
}
