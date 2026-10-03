package node

import (
	"context"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
)

// Node represents a node in the cluster
type Node struct {
	Name  string
	Role  Role
	Stats Stats

	// Runtimes are the classes the node offers, as its last heartbeat said:
	// whether each is healthy there, what it can do and how much room it has.
	// Placement reads them; a node from before there were classes offers none
	// it has said, and runs sysbox.
	Runtimes []runtime.Offer

	LastHeartbeatAt time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Manager represents a manager of nodes. Stats is what the node's runs use
// between them, over every class it offers; what each class offers is
// driver.Set's to say.
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
