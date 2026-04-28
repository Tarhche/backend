package node

import (
	"time"
)

// Node represents a node in the cluster
type Node struct {
	Name            string
	Role            Role
	Stats           Stats
	LastHeartbeatAt time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Manager represents a manager of nodes
type Manager interface {
	Stats(nodeName string) (Stats, error)
}

// Role represents the role of the node
type Role string

const (
	// Worker is a node that runs tasks
	WorkerRole Role = "worker"

	// Manager is a node that manages the cluster
	ManagerRole Role = "manager"
)

// Repository is the interface for the node repository
type Repository interface {
	GetAll(offset uint, limit uint) ([]Node, error)
	GetOne(name string) (Node, error)
	Save(*Node) (string, error)
	Delete(name string) error
	Count() (uint, error)
}
