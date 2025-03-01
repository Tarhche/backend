package nodes

import (
	"time"
)

type NodeBson struct {
	Name            string    `bson:"name"`
	Role            string    `bson:"role"`
	API             string    `bson:"api"`
	Resources       Resource  `bson:"resources"`
	Stats           Stats     `bson:"stats"`
	LastHeartbeatAt time.Time `bson:"last_heartbeat_at,omitempty"`
	CreatedAt       time.Time `bson:"created_at,omitempty"`
	UpdatedAt       time.Time `bson:"updated_at,omitempty"`
}

type Resource struct {
	Cpu    float64 `bson:"cpu"`
	Memory uint64  `bson:"memory"`
	Disk   uint64  `bson:"disk"`
}

type Stats struct {
	Memory Memory `bson:"memory"`
	Disk   Disk   `bson:"disk"`
	CPU    CPU    `bson:"cpu"`
	Load   Load   `bson:"load"`
}

type Memory struct {
	Total     uint64 `bson:"total"`
	Used      uint64 `bson:"used"`
	Available uint64 `bson:"available"`
	SwapTotal uint64 `bson:"swap_total"`
	SwapFree  uint64 `bson:"swap_free"`
}

type Disk struct {
	Total      uint64 `bson:"total"`
	Used       uint64 `bson:"used"`
	Available  uint64 `bson:"available"`
	FreeInodes uint64 `bson:"free_inodes"`
}

type CPU struct {
	ID        string `bson:"id"`
	User      uint64 `bson:"user"`
	Nice      uint64 `bson:"nice"`
	System    uint64 `bson:"system"`
	Idle      uint64 `bson:"idle"`
	IOWait    uint64 `bson:"iowait"`
	IRQ       uint64 `bson:"irq"`
	SoftIRQ   uint64 `bson:"softirq"`
	Steal     uint64 `bson:"steal"`
	Guest     uint64 `bson:"guest"`
	GuestNice uint64 `bson:"guest_nice"`
}

type Load struct {
	Last1Min       float64 `bson:"last_1_min"`
	Last5Min       float64 `bson:"last_5_min"`
	Last15Min      float64 `bson:"last_15_min"`
	ProcessRunning uint64  `bson:"process_running"`
	ProcessTotal   uint64  `bson:"process_total"`
	LastPID        uint64  `bson:"last_pid"`
}
