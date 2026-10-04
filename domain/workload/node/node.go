package node

import (
	"context"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Node represents a node in the cluster
type Node struct {
	Name  string
	Role  Role
	Stats Stats

	// Capacity is what the node's engine offers to VMs and how much of it the
	// VMs it holds have been given, as its last heartbeat said. It is what a
	// VM is placed by.
	Capacity vm.Info

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
	// OrchestratorRole is a node that runs tasks
	OrchestratorRole Role = "orchestrator"

	// ControlPlaneRole is a node that runs the control plane
	ControlPlaneRole Role = "controlplane"
)

// Repository is the interface for the node repository
type Repository interface {
	GetAll(ctx context.Context, offset uint, limit uint) ([]Node, error)
	GetOne(ctx context.Context, name string) (Node, error)
	Save(ctx context.Context, n *Node) (string, error)
	Count(ctx context.Context) (uint, error)
}
