// Package images answers what the control plane asks of a Docker VM's images.
package images

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

// UseCase answers requests about images.
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
	case noderequest.OpImagesList:
		listed, err := daemon.Images(ctx)
		if err != nil {
			return nil, false, err
		}

		images := make([]noderequest.Image, len(listed))
		for n, image := range listed {
			images[n] = noderequest.NewImage(image)
		}

		fitted, truncated := reply.First(images)

		return fitted, truncated, nil

	case noderequest.OpImagesPull:
		var pull noderequest.PullRequest
		if err := reply.Decode(request.Payload, &pull); err != nil {
			return nil, false, err
		}

		if err := reply.Required("reference", pull.Reference); err != nil {
			return nil, false, err
		}

		pulled, err := daemon.PullImage(ctx, pull.Reference)
		if err != nil {
			return nil, false, err
		}

		return noderequest.NewImage(pulled), false, nil

	case noderequest.OpImagesRemove:
		var remove noderequest.RemoveRequest
		if err := reply.Decode(request.Payload, &remove); err != nil {
			return nil, false, err
		}

		if err := reply.Required("id", remove.ID); err != nil {
			return nil, false, err
		}

		return nil, false, daemon.RemoveImage(ctx, remove.ID, remove.Force)
	}

	return nil, false, reply.Invalid("%q is not an operation on images", request.Op)
}
