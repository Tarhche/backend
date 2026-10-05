// Package request carries the control plane's synchronous questions to the
// nodes over core NATS request/reply: the requester asks on the node's own
// subject and waits, and the responder on the node answers.
package request

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	infranats "github.com/khanzadimahdi/testproject/infrastructure/messaging/nats"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

// ErrNoNode is a question no node took: nothing answers on the subject of the
// node it was asked of, which is a node that is down or was never there.
var ErrNoNode = errors.New("no node is answering")

// Requester asks nodes questions and waits for their answers.
//
// How long it waits is the operation's: one that may have to pull an image is
// given the pull timeout and everything else the request timeout, so no caller
// has to know which is which. A caller can still give up sooner, through its
// context.
type Requester struct {
	connection  *nats.Conn
	timeout     time.Duration
	pullTimeout time.Duration
	tracer      oteltrace.Tracer
}

var _ noderequest.Requester = &Requester{}

func NewRequester(connection *nats.Conn, timeout time.Duration, pullTimeout time.Duration) *Requester {
	return &Requester{
		connection:  connection,
		timeout:     timeout,
		pullTimeout: pullTimeout,
		tracer:      otel.Tracer("nats"),
	}
}

// Request asks the named node and waits for its answer. An error is a question
// nobody answered: ErrNoNode when no node of that name is listening, and
// context.DeadlineExceeded when the time ran out. A node that answered and
// refused does so in the Reply.
func (r *Requester) Request(ctx context.Context, nodeName string, request noderequest.Request) (noderequest.Reply, error) {
	subject := noderequest.Subject(nodeName)

	ctx, span := r.tracer.Start(ctx, "nats.request "+noderequest.SubjectPrefix,
		oteltrace.WithSpanKind(oteltrace.SpanKindClient),
		oteltrace.WithAttributes(
			attribute.String("messaging.destination.name", subject),
			attribute.String("workload.node_request.op", string(request.Op)),
			attribute.String("workload.vm", request.VMUUID),
		),
	)
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, r.timeoutOf(request.Op))
	defer cancel()

	payload, err := json.Marshal(request)
	if err != nil {
		return noderequest.Reply{}, trace.RecordError(span, err)
	}

	msg := &nats.Msg{Subject: subject, Data: payload, Header: nats.Header{}}
	otel.GetTextMapPropagator().Inject(ctx, infranats.HeaderCarrier(msg.Header))

	answer, err := r.connection.RequestMsgWithContext(ctx, msg)
	switch {
	case errors.Is(err, nats.ErrNoResponders):
		return noderequest.Reply{}, trace.RecordError(span, fmt.Errorf("%w: %s", ErrNoNode, nodeName))

	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, nats.ErrTimeout):
		return noderequest.Reply{}, trace.RecordError(span, fmt.Errorf("%s did not answer %s in time: %w", nodeName, request.Op, context.DeadlineExceeded))

	case err != nil:
		return noderequest.Reply{}, trace.RecordError(span, err)
	}

	var reply noderequest.Reply
	if err := json.Unmarshal(answer.Data, &reply); err != nil {
		return noderequest.Reply{}, trace.RecordError(span, fmt.Errorf("%s answered %s with something that is not a reply: %w", nodeName, request.Op, err))
	}

	var refused *noderequest.Error
	if errors.As(reply.Err(), &refused) {
		span.SetAttributes(attribute.String("workload.node_request.error", string(refused.Code)))
	}

	return reply, nil
}

// timeoutOf is how long an operation is given.
func (r *Requester) timeoutOf(op noderequest.Op) time.Duration {
	if op.MayPull() {
		return r.pullTimeout
	}

	return r.timeout
}
