package kind

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

// Query is one of a kind's query actions, asked of the node holding the
// resource and answered at once.
//
// It travels as a node request, on the subject every node already answers
// on (noderequest.Subject), rather than on a subject of its own:
//
//   - its Op is the kind and the action, "stack.state" or "vm.logs", the
//     way a VM's log was asked before VMs were a kind. A kind's name and an
//     action's have no dot in them, so an op splits back into the two at its
//     one dot, and one with more than one is never mistaken for a kind's;
//   - its VMUUID is the resource's uuid, whatever its kind: the field is older
//     than kinds, and is what the responder's traces say a request is about;
//   - its payload is the action's own payload and the resource as the control
//     plane recorded it, so a node answers without a database, as it carries
//     out an ActOnResource.
//
// The reply is a noderequest.Reply as it always was.
//
// A command for an instance nobody keeps a record of, one of a kind's extras
// such as a container made from its VM's terminal, travels the same way, and
// its reply is its ResourceActedOn: with no record for a ResourceActedOn to
// be taken onto, whoever asked can only hear what came of it in the answer.
type Query struct {
	Kind   string
	UUID   string
	Action string

	// Payload is the action's own, as its codec reads it.
	Payload json.RawMessage

	// Resource is the manifest as the control plane recorded it.
	Resource Raw
}

// query is what a Query's node request carries as its payload: what the op
// and the uuid do not say already.
type query struct {
	Payload  json.RawMessage `json:"payload,omitempty"`
	Resource Raw             `json:"resource"`
}

// Op is the operation a kind's query action is asked as: "stack.state".
func Op(kindName string, action string) noderequest.Op {
	return noderequest.Op(kindName + "." + action)
}

// ParseOp is the kind and the action an operation asks, and false for one
// that names no kind's action.
func ParseOp(op noderequest.Op) (kindName string, action string, ok bool) {
	kindName, action, found := strings.Cut(string(op), ".")
	if !found || len(kindName) == 0 || len(action) == 0 || strings.Contains(action, ".") {
		return "", "", false
	}

	return kindName, action, true
}

// Request is the query as the node request it travels as.
func (q Query) Request() (noderequest.Request, error) {
	payload, err := json.Marshal(query{Payload: q.Payload, Resource: q.Resource})
	if err != nil {
		return noderequest.Request{}, fmt.Errorf("the %s query cannot be written: %w", Op(q.Kind, q.Action), err)
	}

	return noderequest.Request{Op: Op(q.Kind, q.Action), VMUUID: q.UUID, Payload: payload}, nil
}

// QueryOf reads a kind's query back out of the node request it travelled as.
func QueryOf(request noderequest.Request) (Query, error) {
	kindName, action, ok := ParseOp(request.Op)
	if !ok {
		return Query{}, fmt.Errorf("%w: %q is not a kind's query", ErrUnknownAction, request.Op)
	}

	var asked query
	if err := unmarshal(request.Payload, &asked); err != nil {
		return Query{}, fmt.Errorf("%w: the %s query cannot be read: %w", ErrInvalidPayload, request.Op, err)
	}

	return Query{
		Kind:     kindName,
		UUID:     request.VMUUID,
		Action:   action,
		Payload:  asked.Payload,
		Resource: asked.Resource,
	}, nil
}
