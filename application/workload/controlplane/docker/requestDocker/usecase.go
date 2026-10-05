// Package requestDocker passes a question for a Docker VM's dockerd on to the
// node holding the VM, and its answer back.
//
// Docker's objects are not records anybody keeps: dockerd is the truth about
// its own containers, images, networks and volumes, so they are read from it
// as they are now. The payload goes on as it came, and the answer comes back
// as the node gave it; what the control plane checks is whose VM it is, that
// it is a Docker VM, and that there is a node holding it that can be asked.
package requestDocker

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/ask"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/owner"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type UseCase struct {
	vmRepository vm.Repository
	requester    noderequest.Requester
	validator    domain.Validator
}

func NewUseCase(vmRepository vm.Repository, requester noderequest.Requester, validator domain.Validator) *UseCase {
	return &UseCase{vmRepository: vmRepository, requester: requester, validator: validator}
}

// Execute asks the VM's dockerd, through the node holding it. A VM that is
// still coming up is waited for by its node. A VM that is not there, or not the
// owner's, is domain.ErrNotExists.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	v, err := owner.One(ctx, uc.vmRepository, request.OwnerUUID, request.VMUUID)
	if err != nil {
		return nil, err
	}

	if refused := ask.DockerRefusal(&v); refused != nil {
		return &Response{NodeError: refused}, nil
	}

	reply, refused := ask.Node(ctx, uc.requester, v.NodeName, noderequest.Request{
		Op:      request.Op,
		VMUUID:  v.UUID,
		Payload: request.Payload,
	})
	if refused != nil {
		return &Response{NodeError: refused}, nil
	}

	return &Response{Result: reply.Result, Truncated: reply.Truncated}, nil
}
