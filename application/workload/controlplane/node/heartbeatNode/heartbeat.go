package heartbeatNode

import (
	"context"
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/node/events"
)

type Heartbeat struct {
	nodeRepository node.Repository
}

var _ domain.MessageHandler = &Heartbeat{}

// NewHeartbeatHandler is a handler that writes down what a node says of
// itself in nodeRepository: that it is alive, what it is, what it uses and
// what it offers. What it holds of every kind is heard in the kind's own
// heartbeat (kinds/heartbeatResources).
func NewHeartbeatHandler(nodeRepository node.Repository) *Heartbeat {
	return &Heartbeat{nodeRepository: nodeRepository}
}

func (h *Heartbeat) Handle(ctx context.Context, data []byte) error {
	var heartbeat events.Heartbeat
	if err := json.Unmarshal(data, &heartbeat); err != nil {
		return err
	}

	n, err := h.getNode(ctx, heartbeat.Name)
	if err != nil {
		return err
	}

	n.Name = heartbeat.Name
	n.Role = heartbeat.Role
	n.Stats = heartbeat.Stats
	n.Capacity = heartbeat.Capacity
	n.LastHeartbeatAt = heartbeat.At

	if _, err := h.nodeRepository.Save(ctx, &n); err != nil {
		return err
	}

	return nil
}

func (h *Heartbeat) getNode(ctx context.Context, name string) (node.Node, error) {
	if n, err := h.nodeRepository.GetOne(ctx, name); err == nil {
		return n, nil
	} else if err != nil && err != domain.ErrNotExists {
		return node.Node{}, err
	}

	return node.Node{}, nil
}
