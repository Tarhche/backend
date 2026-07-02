package heartbeatNode

import (
	"context"
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/node"
	"github.com/khanzadimahdi/testproject/domain/runner/node/events"
)

type Heartbeat struct {
	nodeRepository node.Repository
}

var _ domain.MessageHandler = &Heartbeat{}

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
	n.LastHeartbeatAt = heartbeat.At

	_, err = h.nodeRepository.Save(ctx, &n)

	return err
}

func (h *Heartbeat) getNode(ctx context.Context, name string) (node.Node, error) {
	if n, err := h.nodeRepository.GetOne(ctx, name); err == nil {
		return n, nil
	} else if err != nil && err != domain.ErrNotExists {
		return node.Node{}, err
	}

	return node.Node{}, nil
}
