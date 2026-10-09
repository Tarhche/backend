package workload

import (
	"log/slog"

	ingressLocateResources "github.com/khanzadimahdi/testproject/application/workload/ingress/locateResources"
	"github.com/khanzadimahdi/testproject/domain"
	ingressContract "github.com/khanzadimahdi/testproject/domain/workload/ingress"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// IngressWorkload is what the ingress runs of the workload: the kinds whose
// resources it finds, and what it hears of where they are.
type IngressWorkload struct {
	// Kinds are the kinds the ingress finds the resources of, each by its
	// ingress strategy.
	Kinds *kind.Registry[kind.IngressBinding]

	// Subscribers hear where the tasks and the VMs are, to be subscribed to
	// over core NATS: the heartbeats of the nodes holding them, each kind's on
	// its own subject, the commands sent to those nodes, and what came of
	// them.
	Subscribers map[string]domain.MessageHandler
}

// NewIngressWorkload wires the ingress's workload, keeping what it hears in
// locations.
//
// It is the serve command's wiring, and it is what the workload's end-to-end
// test builds an ingress from: what is tested is what is served.
func NewIngressWorkload(locations ingressContract.Locations, logger *slog.Logger) (*IngressWorkload, error) {
	kinds, located, err := ingressKinds(locations)
	if err != nil {
		return nil, err
	}

	subscribers := map[string]domain.MessageHandler{
		// what a command takes away is taken away as it is sent, before its
		// node has carried it out, and what it left is taken as soon as its
		// node says.
		kind.ActOnResourceName:   ingressLocateResources.NewActOnResourceHandler(locations, located, logger),
		kind.ResourceActedOnName: ingressLocateResources.NewResourceActedOnHandler(locations, located, logger),
	}

	// where each task and VM is, as the node holding it says every beat, an
	// instance in each heartbeat, on its kind's own subject: the kinds
	// located are the only ones whose heartbeats the ingress hears.
	heartbeats := ingressLocateResources.NewHeartbeatHandler(locations, located, logger)

	for kindName := range located {
		subscribers[kind.HeartbeatName(kindName)] = heartbeats
	}

	return &IngressWorkload{Kinds: kinds, Subscribers: subscribers}, nil
}
