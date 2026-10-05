// Package networks answers what the control plane asks of a Docker VM's
// networks, none of which reaches past the VM.
package networks

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/internal/reply"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

// Daemons are the dockerds of this node's Docker VMs.
type Daemons interface {
	Daemon(vmUUID string) docker.Daemon
}

// UseCase answers requests about networks.
type UseCase struct {
	daemons Daemons
}

func NewUseCase(daemons Daemons) *UseCase {
	return &UseCase{daemons: daemons}
}

// Answer answers one request, with what the operation answers with and
// whether it had to be cut to fit in a reply.
func (uc *UseCase) Answer(ctx context.Context, request noderequest.Request) (any, bool, error) {
	daemon := uc.daemons.Daemon(request.VMUUID)

	switch request.Op {
	case noderequest.OpNetworksList:
		listed, err := daemon.Networks(ctx)
		if err != nil {
			return nil, false, err
		}

		networks := make([]noderequest.Network, len(listed))
		for n, network := range listed {
			networks[n] = noderequest.NewNetwork(network)
		}

		fitted, truncated := reply.First(networks)

		return fitted, truncated, nil

	case noderequest.OpNetworksCreate:
		var spec noderequest.NetworkSpec
		if err := reply.Decode(request.Payload, &spec); err != nil {
			return nil, false, err
		}

		if err := reply.Required("name", spec.Name); err != nil {
			return nil, false, err
		}

		created, err := daemon.CreateNetwork(ctx, spec.ToDocker())
		if err != nil {
			return nil, false, err
		}

		return noderequest.NewNetwork(created), false, nil

	case noderequest.OpNetworksRemove:
		var remove noderequest.RemoveRequest
		if err := reply.Decode(request.Payload, &remove); err != nil {
			return nil, false, err
		}

		if err := reply.Required("id", remove.ID); err != nil {
			return nil, false, err
		}

		return nil, false, daemon.RemoveNetwork(ctx, remove.ID)
	}

	return nil, false, reply.Invalid("%q is not an operation on networks", request.Op)
}
