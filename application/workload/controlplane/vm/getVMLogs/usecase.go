// Package getVMLogs reads the tail of a VM's log from the node holding it, as
// it is now: nothing keeps it anywhere else.
package getVMLogs

import (
	"context"
	"encoding/json"

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

// Execute is the tail of the VM's log, or why there is none. A VM that is not
// there, or not the owner's, is domain.ErrNotExists.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	v, err := owner.One(ctx, uc.vmRepository, request.OwnerUUID, request.UUID)
	if err != nil {
		return nil, err
	}

	if len(v.NodeName) == 0 {
		return &Response{NodeError: ask.NotRunning()}, nil
	}

	tail := request.Tail
	if tail == 0 || tail > noderequest.MaxLogLines {
		tail = noderequest.MaxLogLines
	}

	reply, refused := ask.Node(ctx, uc.requester, v.NodeName, noderequest.Request{
		Op:      noderequest.OpVMLogs,
		VMUUID:  v.UUID,
		Payload: ask.Payload(noderequest.NewLogsRequest(vm.LogOptions{Since: request.Since, Tail: tail})),
	})
	if refused != nil {
		return &Response{NodeError: refused}, nil
	}

	lines := make([]noderequest.VMLogLine, 0)
	if len(reply.Result) > 0 {
		if err := json.Unmarshal(reply.Result, &lines); err != nil {
			return &Response{NodeError: &noderequest.Error{Code: noderequest.CodeInternal, Message: "the node answered with something that is not a log"}}, nil
		}
	}

	return &Response{Lines: lines, Truncated: reply.Truncated}, nil
}
