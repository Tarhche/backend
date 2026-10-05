// Package createContainer creates a container in a Docker VM: the one the
// request names, the person's only one, or one made for it.
package createContainer

import (
	"context"
	"encoding/json"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/ask"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/dockerVM"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// BootTimeout is how long a Docker VM that is still coming up is waited
	// for before the container is asked of it. A VM made for the container a
	// moment ago is not on its node yet, so the container is asked for once its
	// node reports it running, and its node then waits for dockerd.
	BootTimeout = 5 * time.Minute

	// pollInterval is how often a VM coming up is looked at.
	pollInterval = time.Second
)

type UseCase struct {
	vmRepository vm.Repository
	chooser      *dockerVM.Chooser
	requester    noderequest.Requester
	validator    domain.Validator

	pollInterval time.Duration
}

func NewUseCase(vmRepository vm.Repository, chooser *dockerVM.Chooser, requester noderequest.Requester, validator domain.Validator) *UseCase {
	return &UseCase{
		vmRepository: vmRepository,
		chooser:      chooser,
		requester:    requester,
		validator:    validator,
		pollInterval: pollInterval,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	chosen, refused, err := uc.chooser.Choose(ctx, request.OwnerUUID, request.VM)
	if err != nil {
		return nil, err
	}

	if len(refused) > 0 {
		return &Response{ValidationErrors: refused}, nil
	}

	response := &Response{VM: &presenter.ChosenVM{UUID: chosen.VM.UUID, Name: chosen.VM.Name, Created: chosen.Created}}

	v, notUp, err := uc.up(ctx, chosen.VM)
	if err != nil {
		return nil, err
	}

	if notUp != nil {
		response.NodeError = notUp

		return response, nil
	}

	reply, failed := ask.Node(ctx, uc.requester, v.NodeName, noderequest.Request{
		Op:      noderequest.OpContainersCreate,
		VMUUID:  v.UUID,
		Payload: ask.Payload(request.Container),
	})
	if failed != nil {
		response.NodeError = failed

		return response, nil
	}

	var created noderequest.Container
	if err := json.Unmarshal(reply.Result, &created); err != nil {
		response.NodeError = &noderequest.Error{Code: noderequest.CodeInternal, Message: "the node answered with something that is not a container"}

		return response, nil
	}

	response.Container = &created

	return response, nil
}

// up is the VM once it is running, or why it is not going to be: one that is
// still coming up is waited for, for as long as a VM takes to boot.
func (uc *UseCase) up(ctx context.Context, v vm.VM) (vm.VM, *noderequest.Error, error) {
	ctx, cancel := context.WithTimeout(ctx, BootTimeout)
	defer cancel()

	for {
		if refused := ask.DockerRefusal(&v); refused != nil {
			return v, refused, nil
		}

		if v.CurrentState == vm.Running {
			return v, nil, nil
		}

		select {
		case <-ctx.Done():
			return v, &noderequest.Error{Code: noderequest.CodeTimeout, Message: "the docker vm did not come up in time"}, nil
		case <-time.After(uc.pollInterval):
		}

		fresh, err := uc.vmRepository.GetOne(ctx, v.UUID)
		if err != nil {
			if ctx.Err() != nil {
				return v, &noderequest.Error{Code: noderequest.CodeTimeout, Message: "the docker vm did not come up in time"}, nil
			}

			return v, nil, err
		}

		v = fresh
	}
}
