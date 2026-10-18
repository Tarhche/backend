// Package noderequest is the synchronous half of how the control plane talks
// to a node: a question asked over core NATS request/reply and answered at
// once.
//
// Commands are messages, because what they start takes a while and is
// reported as it happens. A kind's query, a VM's log say, is a request with an
// answer (kind.Query): nothing is stored to read it from. So is a command for
// what nobody keeps a record of, a container made from its VM's terminal say:
// with no record for what came of it to be taken onto, it is answered.
//
// A request names a kind's action and the resource it is about, and carries
// that action's own payload; the reply carries its result or why there is
// none.
package noderequest

import (
	"context"
	"encoding/json"
	"errors"
)

// SubjectPrefix is what every node's requests are asked on, followed by the
// node's name.
const SubjectPrefix = "workloadNodeRequest"

// Subject is what the named node answers requests on.
func Subject(nodeName string) string {
	return SubjectPrefix + "." + nodeName
}

// What a reply may hold. NATS refuses a message over its max_payload, which is
// 1 MiB unless it is configured otherwise, so a reply is kept under it and says
// so when it had to leave something out.
const (
	MaxReplyBytes = 1 << 20

	// MaxLogLines is the most lines of a log a reply carries: the last ones.
	MaxLogLines = 1000
)

// Op is what a request asks for: a kind's action, as kind.Op names it.
type Op string

// Request is one question for a node.
type Request struct {
	Op Op `json:"op"`

	// VMUUID is the resource the question is about, whatever its kind: the
	// field is older than kinds, when every question was about a VM.
	VMUUID string `json:"vm_uuid"`

	// Payload is the operation's own request, as it was given.
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Reply is a node's answer.
type Reply struct {
	OK bool `json:"ok"`

	// Result is the operation's own answer, when it has one.
	Result json.RawMessage `json:"result,omitempty"`

	// Error is why there is no answer, when OK is false.
	Error *Error `json:"error,omitempty"`

	// Truncated says the result was cut to fit in a reply: a log keeps its
	// last lines, a listing its first items.
	Truncated bool `json:"truncated,omitempty"`
}

// errNoReason is a reply that says it failed and not why, which only a node
// that is broken in some other way sends.
var errNoReason = errors.New("the node did not say why the request failed")

// Err is why the request failed, or nil when it did not.
func (r Reply) Err() error {
	if r.OK {
		return nil
	}

	if r.Error == nil {
		return &Error{Code: CodeInternal, Message: errNoReason.Error()}
	}

	return r.Error
}

// Requester asks nodes questions.
type Requester interface {
	// Request asks the named node and waits for its answer for as long as the
	// context allows. An error is a question nobody answered: no node of
	// that name listening, or the time running out. A node that answered and
	// refused does so in the Reply.
	Request(ctx context.Context, nodeName string, request Request) (Reply, error)
}

// Handler answers questions on a node.
type Handler interface {
	// Handle answers one request. It does not fail: whatever goes wrong is the
	// reply's Error, because the node is the only one that can say what it was.
	Handle(ctx context.Context, request Request) Reply
}
