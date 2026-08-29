package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

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
type session struct {
	conn       conn
	dispatcher *dispatcher
	registry   domain.RequestRegistry
	hub        *hub
	bus        *replyBus
	translator translator.Translator
	logger     *slog.Logger

	// pending holds the server-side ids dispatched but not replied to yet.
	pending []string
}

// run drives the session until the client disconnects, then stops taking new
// replies, writes out the ones in hand and closes the connection.
func (s *session) run(ctx context.Context) {
	defer s.conn.shutdown()

	replies, unsubscribe := s.hub.subscribe()

	written := make(chan struct{})
	go func() {
		defer close(written)

		s.writeReplies(replies)
	}()

	s.readRequests(ctx)

	unsubscribe()
	<-written
}

// readRequests reads client requests until the connection breaks or ctx is done.
func (s *session) readRequests(ctx context.Context) {
	defer s.sweepPending()

	for ctx.Err() == nil {
		var request domain.Request

		if err := s.conn.read(&request); err != nil {
			s.logger.ErrorContext(ctx, "error on reading request", "error", err)

			return
		}

		serverSideID, validationErrors, err := s.dispatcher.dispatch(ctx, &request)

		// a registered request must be swept even when dispatching failed
		// afterwards, because the registry entry already exists.
		if len(serverSideID) > 0 {
			s.pending = append(s.pending, serverSideID)
		}

		switch {
		case err != nil:
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
		clientSideID, err := s.registry.GetClientSideID(reply.RequestID)

		switch {
		case errors.Is(err, domain.ErrNotExists):
			s.logger.Warn("request id not found in pending requests")

			continue

		case err != nil:
			s.logger.Error("error on getting client side request id", "error", err)

			// the lookup failed rather than came up empty, so the reply may
			// still be deliverable.
			if !s.bus.requeue(reply) {
				return
			}

			continue
		}

		// the reply is shared with every session, so answer with a copy rather
		// than rewriting the id on it.
		s.conn.send(&domain.Reply{
			RequestID: clientSideID,
			Payload:   reply.Payload,
		})

		s.registry.DeleteByServerSideID(reply.RequestID)
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

// sweepPending drops the registry entries of requests the client disconnected
// before receiving.
func (s *session) sweepPending() {
	for _, serverSideID := range s.pending {
		s.registry.DeleteByServerSideID(serverSideID)
	}

	s.pending = nil
}
