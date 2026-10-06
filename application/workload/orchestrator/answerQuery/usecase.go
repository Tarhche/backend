// Package answerQuery answers what the control plane asks a node about a
// resource of a kind the node runs, and hands every other request on to what
// answered them before there were kinds.
package answerQuery

import (
	"context"
	"errors"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/internal/reply"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

// UseCase answers node requests whose op is a kind's query, "stack.state",
// with that kind's node strategy, and hands every other request on to next.
//
// A kind takes over the ops named after it once it is registered on this
// node, and not before: "vm.logs" is the vm kind's, which every node runs.
// The Docker passthrough's ops, "docker.containers.list", name no kind's
// action and are always the node's.
//
// What a kind's query fails with is the reply's error, in the codes every
// side knows. A query this node cannot ask at all, of an action the kind
// does not have or with a payload that cannot be read, is invalid; anything
// else is the code its error stands for, a strategy's not_found say, and its
// words are the message.
type UseCase struct {
	kinds *kind.Registry[kind.NodeBinding]
	next  noderequest.Handler
}

var _ noderequest.Handler = &UseCase{}

func NewUseCase(kinds *kind.Registry[kind.NodeBinding], next noderequest.Handler) *UseCase {
	return &UseCase{kinds: kinds, next: next}
}

func (uc *UseCase) Handle(ctx context.Context, request noderequest.Request) noderequest.Reply {
	binding, ok := uc.binding(request.Op)
	if !ok {
		return uc.next.Handle(ctx, request)
	}

	// the field is older than kinds, and is the resource's uuid whatever its
	// kind.
	if err := reply.Required("vm_uuid", request.VMUUID); err != nil {
		return noderequest.Failed(err)
	}

	query, err := kind.QueryOf(request)
	if err != nil {
		return failed(err)
	}

	answer, err := binding.Query(ctx, query)
	if err != nil {
		return failed(err)
	}

	return noderequest.Reply{OK: true, Result: answer}
}

// binding is the kind an op asks a query of, when it names a kind that is
// registered here.
func (uc *UseCase) binding(op noderequest.Op) (kind.NodeBinding, bool) {
	kindName, _, ok := kind.ParseOp(op)
	if !ok {
		return nil, false
	}

	return uc.kinds.Lookup(kindName)
}

// failed is the reply to a query that could not be answered because of err.
func failed(err error) noderequest.Reply {
	if errors.Is(err, kind.ErrUnknownKind) || errors.Is(err, kind.ErrUnknownAction) || errors.Is(err, kind.ErrInvalidPayload) {
		return noderequest.Failed(&noderequest.Error{Code: noderequest.CodeInvalid, Message: err.Error()})
	}

	return noderequest.Failed(err)
}
