// Package volumes answers what the control plane asks of a Docker VM's
// volumes.
package volumes

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

// UseCase answers requests about volumes. A volume is named by its name,
// which is the only id it has.
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
	case noderequest.OpVolumesList:
		listed, err := daemon.Volumes(ctx)
		if err != nil {
			return nil, false, err
		}

		volumes := make([]noderequest.Volume, len(listed))
		for n, volume := range listed {
			volumes[n] = noderequest.NewVolume(volume)
		}

		fitted, truncated := reply.First(volumes)

		return fitted, truncated, nil

	case noderequest.OpVolumesCreate:
		// a volume asked for with no name is named by docker.
		var spec noderequest.VolumeSpec
		if err := reply.Decode(request.Payload, &spec); err != nil {
			return nil, false, err
		}

		created, err := daemon.CreateVolume(ctx, spec.ToDocker())
		if err != nil {
			return nil, false, err
		}

		return noderequest.NewVolume(created), false, nil

	case noderequest.OpVolumesRemove:
		var remove noderequest.RemoveRequest
		if err := reply.Decode(request.Payload, &remove); err != nil {
			return nil, false, err
		}

		if err := reply.Required("id", remove.ID); err != nil {
			return nil, false, err
		}

		return nil, false, daemon.RemoveVolume(ctx, remove.ID, remove.Force)
	}

	return nil, false, reply.Invalid("%q is not an operation on volumes", request.Op)
}
