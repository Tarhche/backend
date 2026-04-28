package nodes

import (
	"time"
)

type NodeBson struct {
	Name            string    `bson:"name"`
	Role            string    `bson:"role"`
	API             string    `bson:"api"`
	Stats           Stats     `bson:"stats"`
	LastHeartbeatAt time.Time `bson:"last_heartbeat_at,omitempty"`
	CreatedAt       time.Time `bson:"created_at,omitempty"`
	UpdatedAt       time.Time `bson:"updated_at,omitempty"`
}

type Stats struct {
	PIDs          uint64  `bson:"pids"`
	CPUPercent    float64 `bson:"cpu_percent"`
	MemoryUsage   uint64  `bson:"memory_usage"`
	MemoryLimit   uint64  `bson:"memory_limit,omitempty"`
	MemoryPercent float64 `bson:"memory_percent"`
	NetworkInput  uint64  `bson:"network_input"`
	NetworkOutput uint64  `bson:"network_output"`
	BlockInput    uint64  `bson:"block_input"`
	BlockOutput   uint64  `bson:"block_output"`
}
