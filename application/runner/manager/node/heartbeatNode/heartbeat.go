package heartbeatNode

import (
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

func (h *Heartbeat) Handle(data []byte) error {
	var heartbeat events.Heartbeat
	if err := json.Unmarshal(data, &heartbeat); err != nil {
		return err
	}

	n, err := h.getNode(heartbeat.Name)
	if err != nil {
		return err
	}

	n.Name = heartbeat.Name
	n.Role = heartbeat.Role
	n.Resources = heartbeat.Resources
	n.Stats = heartbeat.Stats
	n.LastHeartbeatAt = heartbeat.At

	_, err = h.nodeRepository.Save(&n)

	return err
}

func (h *Heartbeat) getNode(name string) (node.Node, error) {
	if n, err := h.nodeRepository.GetOne(name); err == nil {
		return n, nil
	} else if err != nil && err != domain.ErrNotExists {
		return node.Node{}, err
	}

	return node.Node{}, nil
}
