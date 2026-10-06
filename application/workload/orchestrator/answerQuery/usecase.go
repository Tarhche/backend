// Package answerQuery answers what the control plane asks a node about a
// resource of a kind the node runs, and hands every other request on to what
// answered them before there were kinds.
package answerQuery

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/internal/reply"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

// Locks keeps what is done to one resource from overlapping, as the node's
// commands take turns at a resource.
type Locks interface {
	// Lock holds the lock of the resource uuid names, waiting for whoever
	// holds it now, and hands back what releases it. Giving up on waiting
	// is ctx ending, which holds nothing.
	Lock(ctx context.Context, uuid string) (release func(), err error)
}

// UseCase answers node requests whose op is a kind's query, "stack.state",
// with that kind's node strategy, and hands every other request on to next.
//
// A kind takes over the ops named after it once it is registered on this
// node, and not before: "vm.logs" is the vm kind's, which every node runs.
// An op that names no kind's action, or one of a kind not run here, is
// next's to answer, and with no next it is not an operation this node
// answers.
//
// A command asked as a request is carried out, under its resource's lock as
// one sent on workloadCommand is, and answered with its Result: it is how the
// control plane asks something of an instance nobody keeps a record of, such
// as a container made from its VM's terminal, since nothing would hear its
// Result on workloadResult.
//
// What a kind's query fails with is the reply's error, in the codes every
// side knows. A query this node cannot ask at all, of an action the kind
// does not have or with a payload that cannot be read, is invalid; anything
// else is the code its error stands for, a strategy's not_found say, and its
// words are the message.
type UseCase struct {
	kinds *kind.Registry[kind.NodeBinding]
	next  noderequest.Handler
	locks Locks
}

var _ noderequest.Handler = &UseCase{}

// Option changes how requests are answered.
type Option func(*UseCase)

// WithLocks has a command asked as a request wait for whatever else is being
// done to its resource, and hold it off while it is carried out.
func WithLocks(locks Locks) Option {
	return func(uc *UseCase) {
		uc.locks = locks
	}
}

func NewUseCase(kinds *kind.Registry[kind.NodeBinding], next noderequest.Handler, options ...Option) *UseCase {
	uc := &UseCase{kinds: kinds, next: next}

	for _, option := range options {
		option(uc)
	}

	return uc
}

func (uc *UseCase) Handle(ctx context.Context, request noderequest.Request) noderequest.Reply {
	binding, ok := uc.binding(request.Op)
	if !ok {
		if uc.next == nil {
			return noderequest.Failed(reply.Invalid("%q is not an operation a node answers", request.Op))
		}

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

	if action, found := binding.Descriptor().Action(query.Action); found && action.Mode == kind.ModeCommand {
		return uc.command(ctx, binding, query)
	}

	answer, err := binding.Query(ctx, query)
	if err != nil {
		return failed(err)
	}

	return noderequest.Reply{OK: true, Result: answer}
}

// command carries out a command asked as a request, and answers with what
// came of it: a command that failed is answered as well as one that did not,
// its Result saying so.
func (uc *UseCase) command(ctx context.Context, binding kind.NodeBinding, query kind.Query) noderequest.Reply {
	if uc.locks != nil {
		release, err := uc.locks.Lock(ctx, query.UUID)
		if err != nil {
			return noderequest.Failed(err)
		}
		defer release()
	}

	result := binding.Execute(ctx, kind.Command{
		Kind:     query.Kind,
		UUID:     query.UUID,
		Action:   query.Action,
		Node:     query.Resource.Metadata.Node,
		Payload:  query.Payload,
		Resource: query.Resource,
	})
	result.At = time.Now()

	answer, err := json.Marshal(result)
	if err != nil {
		return noderequest.Failed(err)
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
