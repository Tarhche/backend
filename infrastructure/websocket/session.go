package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
)

// failureResponse is sent when a request never reaches the queue.
type failureResponse struct {
	Error            string                  `json:"error,omitempty"`
	ValidationErrors domain.ValidationErrors `json:"validationErrors,omitempty"`
}

// conn is the transport a session talks over. It carries whole messages in both
// directions; framing and keepalive are the transport's business.
type conn interface {
	// read blocks until the next client message is decoded into value.
	read(value any) error

	// send reports whether the transport took the message. It must not block on
	// a peer that is not reading.
	send(value any) bool

	// shutdown stops the transport and releases the connection.
	shutdown() error
}

// session is one client's conversation with the server: it reads requests, hands
// them to the dispatcher, and writes back the replies for what it dispatched.
//
// Its registry is its own, so the client-side ids one client chooses cannot
// collide with another's, and a session can only ever resolve a reply to a
// request it registered itself.
type session struct {
	conn         conn
	dispatcher   *dispatcher
	registry     RequestRegistry
	hub          *hub
	bus          *replyBus
	replyBackoff Backoff
	queueBackoff Backoff
	translator   translator.Translator
	logger       *slog.Logger

	// done is closed when the client is gone, so work being retried on behalf
	// of that client stops instead of being waited out. run owns it.
	done chan struct{}
}

// run drives the session until the client disconnects, then stops taking new
// replies, writes out the ones in hand and closes the connection.
func (s *session) run(ctx context.Context) {
	s.done = make(chan struct{})

	defer s.conn.shutdown()

	replies, unsubscribe := s.hub.subscribe()

	written := make(chan struct{})
	go func() {
		defer close(written)

		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Error("recovered a panic while writing replies", "panic", recovered, "stack", string(debug.Stack()))
			}
		}()

		s.writeReplies(replies)
	}()

	// deferred, so that a panic in the read loop still unwinds the session:
	// leaving the hub subscribed would block writeReplies on a channel nobody
	// closes, stranding the goroutine and everything it holds. Defers run in
	// reverse, so this still happens before the connection is shut down.
	defer func() {
		close(s.done)
		unsubscribe()
		<-written
	}()

	s.readRequests(ctx)
}

// readRequests reads client requests until the connection breaks or ctx is done.
func (s *session) readRequests(ctx context.Context) {
	for ctx.Err() == nil {
		var request domain.Request

		if err := s.conn.read(&request); err != nil {
			s.logger.ErrorContext(ctx, "error on reading request", "error", err)

			return
		}

		if s.bus.isClosed() {
			s.writeFailure(request.ID, nil, ErrClosed)

			continue
		}

		serverSideID, validationErrors, err := s.dispatcher.dispatch(ctx, &request)

		switch {
		case err != nil:
			if len(serverSideID) > 0 {
				if deleteErr := s.registry.DeleteByServerSideID(serverSideID); deleteErr != nil {
					s.logger.ErrorContext(ctx, "error on removing a failed request from the registry", "error", deleteErr)
				}
			}

			s.writeFailure(request.ID, nil, err)
		case len(validationErrors) > 0:
			s.writeFailure(request.ID, validationErrors, nil)
		}
	}
}

// writeReplies writes out the replies addressed to this session's requests,
// leaving the rest to the session that registered them.
func (s *session) writeReplies(replies <-chan *domain.Reply) {
	for reply := range replies {
		s.deliver(reply)
	}
}

// deliver writes one reply to the client that is waiting for it. A reply this
// session does not own belongs to another connection and is left alone. A
// lookup that fails and a client whose queue is full are retried on their own
// backoffs until those run out.
func (s *session) deliver(reply *domain.Reply) {
	var lookupAttempt, queueAttempt int

	for {
		clientSideID, err := s.registry.GetClientSideID(reply.RequestID)

		var (
			backoff Backoff
			attempt int
		)

		switch {
		case errors.Is(err, domain.ErrNotExists):
			return

		case err != nil:
			lookupAttempt++
			s.logger.Error("error on getting client side request id", "error", err, "attempt", lookupAttempt)

			backoff, attempt = s.replyBackoff, lookupAttempt

		default:
			// the reply is shared with every session, so answer with a copy
			// rather than rewriting the id on it.
			if s.conn.send(&domain.Reply{RequestID: clientSideID, Payload: reply.Payload}) {
				s.registry.DeleteByServerSideID(reply.RequestID)

				return
			}

			// the client's queue was full. Leave the request registered so the
			// next attempt can still address it, rather than dropping both the
			// reply and the means to deliver it.
			queueAttempt++
			s.logger.Warn("client queue is full, retrying the reply", "requestID", reply.RequestID, "attempt", queueAttempt)

			backoff, attempt = s.queueBackoff, queueAttempt
		}

		wait, retry := backoff.Next(attempt)
		if !retry {
			s.logger.Error("giving up on routing a reply", "requestID", reply.RequestID, "attempts", attempt)

			return
		}

		select {
		case <-time.After(wait):
		case <-s.done:
			// this client is gone; there is nobody left to deliver to.
			return
		case <-s.bus.closed():
			return
		}
	}
}

// writeFailure tells the client its request was rejected. A server-side error is
// reported without its details, which belong in the logs.
func (s *session) writeFailure(requestID string, validationErrors domain.ValidationErrors, err error) {
	s.logger.Warn("writing failure response to client", "requestID", requestID, "validationErrors", validationErrors, "error", err)

	response := &failureResponse{
		ValidationErrors: validationErrors,
	}

	if err != nil {
		response.Error = s.translator.Translate("error_on_processing_the_request")
	}

	payload, marshalErr := json.Marshal(response)
	if marshalErr != nil {
		s.logger.Error("error marshalling failure payload", "error", marshalErr)

		return
	}

	s.conn.send(&domain.Reply{
		RequestID: requestID,
		Payload:   payload,
	})
}
