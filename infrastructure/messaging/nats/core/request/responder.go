// Package request carries the questions the control plane asks a node and
// waits for, over core NATS request/reply.
package request

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	infranats "github.com/khanzadimahdi/testproject/infrastructure/messaging/nats"
)

// ResponderOptions say how many requests a node answers at once and how long
// it gives each.
type ResponderOptions struct {
	// Concurrency is how many requests are answered at once. Past it, a
	// request waits for one to finish, which is what keeps a burst of them
	// from taking every connection into the node's VMs at once.
	Concurrency int

	// Timeout is how long answering a request may take.
	Timeout time.Duration
}

// Responder answers the requests asked of one node.
//
// What a request asks is done on a context of its own rather than the
// caller's: the caller waits only so long, and a command it gave up on
// halfway, a container stopped say, is still worth finishing, since what it
// reads next shows what came of it. So the work is bounded by its own
// timeout, never cut short by the caller leaving, and the reply goes to
// nobody when nobody is waiting any more.
type Responder struct {
	connection *nats.Conn
	handler    noderequest.Handler
	options    ResponderOptions
	logger     *slog.Logger
	tracer     oteltrace.Tracer

	slots chan struct{}
	wg    sync.WaitGroup
}

// NewResponder answers requests with handler.
func NewResponder(connection *nats.Conn, handler noderequest.Handler, options ResponderOptions, logger *slog.Logger) *Responder {
	return &Responder{
		connection: connection,
		handler:    handler,
		options:    options,
		logger:     logger,
		tracer:     otel.Tracer("nats.request"),
		slots:      make(chan struct{}, max(options.Concurrency, 1)),
	}
}

// Serve answers what is asked on subject until ctx ends.
func (r *Responder) Serve(ctx context.Context, subject string) error {
	subscription, err := r.connection.Subscribe(subject, r.receive)
	if err != nil {
		return err
	}

	r.wg.Add(1)

	go func() {
		defer r.wg.Done()

		<-ctx.Done()
		_ = subscription.Drain()
	}()

	return nil
}

// Wait waits for the requests being answered to be answered, once serving has
// stopped.
func (r *Responder) Wait() {
	r.wg.Wait()
}

// receive takes one request, waiting for a slot to answer it in. Messages are
// handed over one at a time, so waiting here holds back the next rather than
// letting them pile up in memory.
func (r *Responder) receive(message *nats.Msg) {
	r.slots <- struct{}{}
	r.wg.Add(1)

	go func() {
		defer r.wg.Done()
		defer func() { <-r.slots }()

		r.answer(message)
	}()
}

func (r *Responder) answer(message *nats.Msg) {
	// the caller's span context arrives in the headers, and answering is a
	// span of the same trace: the caller is waiting on it.
	remote := otel.GetTextMapPropagator().Extract(context.Background(), infranats.HeaderCarrier(message.Header))

	ctx, span := r.tracer.Start(remote, "noderequest.answer",
		oteltrace.WithSpanKind(oteltrace.SpanKindServer),
		oteltrace.WithAttributes(attribute.String("messaging.destination.name", message.Subject)),
	)
	defer span.End()

	var request noderequest.Request

	reply := func() noderequest.Reply {
		if err := json.Unmarshal(message.Data, &request); err != nil {
			return noderequest.Failed(&noderequest.Error{Code: noderequest.CodeInvalid, Message: "the request cannot be read: " + err.Error()})
		}

		span.SetAttributes(
			attribute.String("workload.noderequest.op", string(request.Op)),
			attribute.String("workload.vm", request.VMUUID),
		)

		work, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.options.Timeout)
		defer cancel()

		return r.handler.Handle(work, request)
	}()

	if reply.Error != nil {
		span.SetAttributes(attribute.String("workload.noderequest.error", string(reply.Error.Code)))
	}

	payload, err := json.Marshal(reply)
	if err == nil && len(payload) > noderequest.MaxReplyBytes {
		payload, err = json.Marshal(noderequest.Failed(&noderequest.Error{Code: noderequest.CodeInternal, Message: "the answer does not fit in a reply"}))
	}

	if err != nil {
		r.logger.ErrorContext(ctx, "a node request's reply could not be encoded", "error", err, "op", request.Op)

		return
	}

	if err := message.Respond(payload); err != nil {
		r.logger.WarnContext(ctx, "a node request could not be replied to", "error", err, "op", request.Op)
	}
}
