package nodes

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type NodeBson struct {
	Name  string `bson:"name"`
	Role  string `bson:"role"`
	Stats Stats  `bson:"stats"`

	// Capacity is what the node's engine offers to VMs and how much of it is
	// given, as its last VM heartbeat said.
	Capacity Capacity `bson:"capacity"`

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

// Capacity is a vm.Info as it is stored: whole vCPUs, and bytes.
type Capacity struct {
	Engine    string    `bson:"engine"`
	Version   string    `bson:"version"`
	CPUs      uint      `bson:"cpus"`
	Memory    uint64    `bson:"memory"`
	Disk      uint64    `bson:"disk"`
	Allocated Allocated `bson:"allocated"`
}

type Allocated struct {
	CPUs   uint   `bson:"cpus"`
	Memory uint64 `bson:"memory"`
	Disk   uint64 `bson:"disk"`
}

func toCapacity(info vm.Info) Capacity {
	return Capacity{
		Engine:  info.Engine,
		Version: info.Version,
		CPUs:    info.CPUs,
		Memory:  info.Memory,
		Disk:    info.Disk,
		Allocated: Allocated{
			CPUs:   info.Allocated.CPUs,
			Memory: info.Allocated.Memory,
			Disk:   info.Allocated.Disk,
		},
	}
}

func (c Capacity) toInfo() vm.Info {
	return vm.Info{
		Engine:  c.Engine,
		Version: c.Version,
		CPUs:    c.CPUs,
		Memory:  c.Memory,
		Disk:    c.Disk,
		Allocated: vm.Resources{
			CPUs:   c.Allocated.CPUs,
			Memory: c.Allocated.Memory,
			Disk:   c.Allocated.Disk,
		},
	}
}
