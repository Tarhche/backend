package node

import (
	"context"
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
	Stats(ctx context.Context, nodeName string) (Stats, error)
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
	GetAll(ctx context.Context, offset uint, limit uint) ([]Node, error)
	GetOne(ctx context.Context, name string) (Node, error)
	Save(ctx context.Context, n *Node) (string, error)
	Count(ctx context.Context) (uint, error)
}
