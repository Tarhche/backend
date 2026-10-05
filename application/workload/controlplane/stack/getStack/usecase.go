// Package getStack reads one stack, and the containers compose made for it from
// the dockerd of the VM it is in.
package getStack

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/ask"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/owner"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type UseCase struct {
	stackRepository stack.Repository
	vmRepository    vm.Repository
	requester       noderequest.Requester
	logger          *slog.Logger
}

func NewUseCase(stackRepository stack.Repository, vmRepository vm.Repository, requester noderequest.Requester, logger *slog.Logger) *UseCase {
	return &UseCase{stackRepository: stackRepository, vmRepository: vmRepository, requester: requester, logger: logger}
}

// Execute is the stack and its containers. The record is the answer even when
// its VM cannot be asked: the containers are then none, and VMNotRunning says
// why. A stack that is not there, or not the owner's, is domain.ErrNotExists.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	s, err := owner.One(ctx, uc.stackRepository, request.OwnerUUID, request.UUID)
	if err != nil {
		return nil, err
	}

	response := &Response{Stack: presenter.NewStack(&s), Containers: []presenter.Container{}}

	v, err := uc.vmRepository.GetOne(ctx, s.VMUUID)
	if errors.Is(err, domain.ErrNotExists) {
		response.VMNotRunning = true

		return response, nil
	} else if err != nil {
		return nil, err
	}

	if ask.DockerRefusal(&v) != nil || v.CurrentState != vm.Running {
		response.VMNotRunning = true

		return response, nil
	}

	reply, refused := ask.Node(ctx, uc.requester, v.NodeName, noderequest.Request{
		Op:      noderequest.OpContainersList,
		VMUUID:  v.UUID,
		Payload: ask.Payload(noderequest.NewContainersRequest(docker.ContainerFilter{All: true, Stack: s.Slug})),
	})
	if refused != nil {
		uc.logger.WarnContext(ctx, "a stack's containers could not be listed", "stack", s.UUID, "vm", v.UUID, "code", refused.Code, "error", refused.Message)

		response.VMNotRunning = errors.Is(refused, vm.ErrNotRunning) || errors.Is(refused, docker.ErrUnavailable)

		return response, nil
	}

	if err := json.Unmarshal(reply.Result, &response.Containers); err != nil || response.Containers == nil {
		response.Containers = []presenter.Container{}
	}

	return response, nil
}
