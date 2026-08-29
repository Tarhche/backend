package routing

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/protocol"
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/transport"
)

// Conn is the transport a session talks over. It carries whole messages in both
// directions; framing and keepalive are the transport's business.
type Conn interface {
	// Read blocks until the next client message is decoded into value.
	Read(value any) error

	// Send reports whether the transport took the message. It must not block on
	// a peer that is not reading.
	Send(value any) bool

	// Shutdown stops the transport and releases the connection.
	Shutdown() error
}

// Session is one client's conversation with the server: it reads requests, hands
// them to the dispatcher, and writes back the replies for what it dispatched.
//
// Its registry is its own, so the client-side ids one client chooses cannot
// collide with another's, and a session can only ever resolve a reply to a
// request it registered itself.
type Session struct {
	conn         Conn
	dispatcher   *Dispatcher
	registry     RequestRegistry
	hub          *transport.Hub
	bus          *transport.ReplyBus
	replyBackoff Backoff
	queueBackoff Backoff
	translator   translator.Translator
	logger       *slog.Logger

	// done is closed when the client is gone, so work being retried on behalf
	// of that client stops instead of being waited out. run owns it.
	done chan struct{}
}

// Run drives the session until the client disconnects, then stops taking new
// replies, writes out the ones in hand and closes the connection.
func (s *Session) Run(ctx context.Context) {
	s.done = make(chan struct{})

	defer s.conn.Shutdown()

	replies, unsubscribe := s.hub.Subscribe()

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
func (s *Session) readRequests(ctx context.Context) {
	for ctx.Err() == nil {
		var request domain.Request

		if err := s.conn.Read(&request); err != nil {
			s.logger.ErrorContext(ctx, "error on reading request", "error", err)

			return
		}

		if s.bus.IsClosed() {
			s.writeFailure(request.ID, nil, transport.ErrClosed)

			continue
		}

		serverSideID, validationErrors, err := s.dispatcher.Dispatch(ctx, &request)

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
func (s *Session) writeReplies(replies <-chan *domain.Reply) {
	for reply := range replies {
		s.deliver(reply)
	}
}

// deliver writes one reply to the client that is waiting for it. A reply this
// session does not own belongs to another connection and is left alone. A
// lookup that fails and a client whose queue is full are retried on their own
// backoffs until those run out.
func (s *Session) deliver(reply *domain.Reply) {
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
			if s.conn.Send(&domain.Reply{RequestID: clientSideID, Payload: reply.Payload}) {
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
		case <-s.bus.Closed():
			return
		}
	}
}

// writeFailure tells the client its request was rejected. A server-side error is
// reported without its details, which belong in the logs.
func (s *Session) writeFailure(requestID string, validationErrors domain.ValidationErrors, err error) {
	s.logger.Warn("writing failure response to client", "requestID", requestID, "validationErrors", validationErrors, "error", err)

	response := &protocol.FailureResponse{
		ValidationErrors: validationErrors,
	}

	if err != nil {
		response.Error = s.translator.Translate(protocol.ErrorOnProcessingMessage)
	}

	payload, marshalErr := json.Marshal(response)
	if marshalErr != nil {
		s.logger.Error("error marshalling failure payload", "error", marshalErr)

		return
	}

	s.conn.Send(&domain.Reply{
		RequestID: requestID,
		Payload:   payload,
	})
}
