package node

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/stats"
)

// Node represents a node in the cluster
type Node struct {
	Name            string
	Role            Role
	Resources       Resource
	Stats           stats.Stats
	LastHeartbeatAt time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Resource represents the hardware resources of the node
type Resource struct {
	Cpu    float64
	Memory uint64
	Disk   uint64
}

// Role represents the role of the node
type Role string

const (
	// Worker is a node that runs tasks
	Worker Role = "worker"

	// Manager is a node that manages the cluster
	Manager Role = "manager"
)

// Repository is the interface for the node repository
type Repository interface {
	GetAll(offset uint, limit uint) ([]Node, error)
	GetOne(name string) (Node, error)
	Save(*Node) (string, error)
	Delete(name string) error
	Count() (uint, error)
}
