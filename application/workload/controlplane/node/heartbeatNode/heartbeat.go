// Package heartbeatNode hears what a node says, every beat, about itself
// and, kind by kind, about the resources it holds.
package heartbeatNode

import (
	"context"
	"encoding/json"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/node/events"
)

// Observer writes down what a node's heartbeat says of the resources of
// every kind the node holds: each kind's report, by kind. It fails nothing:
// the next beat says it all again.
type Observer interface {
	Heartbeat(ctx context.Context, nodeName string, at time.Time, reports map[string]kind.Report[json.RawMessage])
}

type Heartbeat struct {
	nodeRepository node.Repository
	observer       Observer
}

var _ domain.MessageHandler = &Heartbeat{}

// NewHeartbeatHandler is a handler that writes down what a node says of
// itself in nodeRepository, and hands what it observed of the resources it
// holds to observer. An observer of nil hears none of it.
func NewHeartbeatHandler(nodeRepository node.Repository, observer Observer) *Heartbeat {
	return &Heartbeat{nodeRepository: nodeRepository, observer: observer}
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

	if _, err := h.nodeRepository.Save(ctx, &n); err != nil {
		return err
	}

	// a node that runs no kinds yet reports none, as every heartbeat sent
	// before kinds were does.
	if h.observer != nil && len(heartbeat.Observations) > 0 {
		at := heartbeat.At
		if at.IsZero() {
			at = time.Now()
		}

		h.observer.Heartbeat(ctx, heartbeat.Name, at, heartbeat.Observations)
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
