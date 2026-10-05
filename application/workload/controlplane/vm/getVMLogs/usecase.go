// Package getVMLogs reads the tail of a VM's log from the node holding it, as
// it is now: nothing keeps it anywhere else. One of the code runner's runs is
// read from what its task keeps of its output instead.
package getVMLogs

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/ask"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/owner"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/coderunner"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type UseCase struct {
	vmRepository vm.Repository
	runs         *coderunner.Runs
	requester    noderequest.Requester
	validator    domain.Validator

	now func() time.Time
}

func NewUseCase(vmRepository vm.Repository, runs *coderunner.Runs, requester noderequest.Requester, validator domain.Validator) *UseCase {
	return &UseCase{vmRepository: vmRepository, runs: runs, requester: requester, validator: validator, now: time.Now}
}

// Execute is the tail of the VM's log, or why there is none: a VM on no node,
// or one its node has not made yet, is not running. A VM that is not there, or
// not the owner's, is domain.ErrNotExists.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	v, err := owner.One(ctx, uc.vmRepository, request.OwnerUUID, request.UUID)
	if errors.Is(err, domain.ErrNotExists) {
		return uc.runLogs(ctx, request, err)
	} else if err != nil {
		return nil, err
	}

	if len(v.NodeName) == 0 {
		return &Response{NodeError: ask.NotRunning()}, nil
	}

	if refused := ask.NotHeld(&v, uc.now()); refused != nil {
		return &Response{NodeError: refused}, nil
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

// runLogs is what the run a uuid that names no VM may name has written.
// notThere is what looking for a VM came to, which is the answer when it names
// no run either.
func (uc *UseCase) runLogs(ctx context.Context, request *Request, notThere error) (*Response, error) {
	run, err := uc.runs.One(ctx, request.OwnerUUID, request.UUID)
	if errors.Is(err, domain.ErrNotExists) {
		return nil, notThere
	} else if err != nil {
		return nil, err
	}

	lines, truncated := coderunner.Logs(&run, vm.LogOptions{Since: request.Since, Tail: request.Tail})

	return &Response{Lines: lines, Truncated: truncated}, nil
}
