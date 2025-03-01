package getNode

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/node"
)

type Response struct {
	Name            string    `json:"name"`
	Resources       Resource  `json:"resources"`
	Stats           Stats     `json:"stats"`
	LastHeartbeatAt time.Time `json:"last_heartbeat_at"`
	CreatedAt       time.Time `json:"created_at"`
}

type Resource struct {
	Cpu    float64 `json:"cpu"`
	Memory uint64  `json:"memory"`
	Disk   uint64  `json:"disk"`
}

type Stats struct {
	Memory Memory `json:"memory"`
	Disk   Disk   `json:"disk"`
	CPU    CPU    `json:"cpu"`
	Load   Load   `json:"load"`
}

type Memory struct {
	Total     uint64 `json:"total"`
	Used      uint64 `json:"used"`
	Available uint64 `json:"available"`
}

type Disk struct {
	Total      uint64 `json:"total"`
	Used       uint64 `json:"used"`
	Available  uint64 `json:"available"`
	FreeInodes uint64 `json:"free_inodes"`
}

type CPU struct {
	User      uint64 `json:"user"`
	Nice      uint64 `json:"nice"`
	System    uint64 `json:"system"`
	Idle      uint64 `json:"idle"`
	IOWait    uint64 `json:"io_wait"`
	IRQ       uint64 `json:"irq"`
	SoftIRQ   uint64 `json:"soft_irq"`
	Steal     uint64 `json:"steal"`
	Guest     uint64 `json:"guest"`
	GuestNice uint64 `json:"guest_nice"`
}

type Load struct {
	Last1Min       float64 `json:"last_1_min"`
	Last5Min       float64 `json:"last_5_min"`
	Last15Min      float64 `json:"last_15_min"`
	ProcessRunning uint64  `json:"process_running"`
	ProcessTotal   uint64  `json:"process_total"`
	LastPID        uint64  `json:"last_pid"`
}

func NewResponse(node *node.Node) *Response {
	return &Response{
		Name: node.Name,
		Resources: Resource{
			Cpu:    node.Resources.Cpu,
			Memory: node.Resources.Memory,
			Disk:   node.Resources.Disk,
		},
		Stats: Stats{
			Memory: Memory{
				Total:     node.Stats.Memory.Total,
				Used:      node.Stats.Memory.Used,
				Available: node.Stats.Memory.Available,
			},
			Disk: Disk{
				Total:      node.Stats.Disk.Total,
				Used:       node.Stats.Disk.Used,
				Available:  node.Stats.Disk.Available,
				FreeInodes: node.Stats.Disk.FreeInodes,
			},
			CPU: CPU{
				User:   node.Stats.CPU.User,
				Nice:   node.Stats.CPU.Nice,
				System: node.Stats.CPU.System,
				Idle:   node.Stats.CPU.Idle,
				IOWait: node.Stats.CPU.IOWait,
			},
			Load: Load{
				Last1Min:       node.Stats.Load.Last1Min,
				Last5Min:       node.Stats.Load.Last5Min,
				Last15Min:      node.Stats.Load.Last15Min,
				ProcessRunning: node.Stats.Load.ProcessRunning,
				ProcessTotal:   node.Stats.Load.ProcessTotal,
				LastPID:        node.Stats.Load.LastPID,
			},
		},
		LastHeartbeatAt: node.LastHeartbeatAt,
		CreatedAt:       node.CreatedAt,
	}
}
