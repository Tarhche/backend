package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
)

// failureResponse is sent when a request never reaches the queue.
type failureResponse struct {
	Error            string                  `json:"error,omitempty"`
	ValidationErrors domain.ValidationErrors `json:"validationErrors,omitempty"`
}

// session is one client's conversation with the server: it reads requests,
// hands them to the dispatcher, and writes back the replies for what it
// dispatched.
type session struct {
	conn       Conn
	dispatcher *dispatcher
	registry   RequestRegistry

	// cancelStream tells whoever is producing a stream that this client has
	// stopped listening to it.
	cancelStream func(ctx context.Context, requestID string)

	hub          *hub
	bus          *replyBus
	replyBackoff Backoff
	queueBackoff Backoff
	translator   translator.Translator
	logger       *slog.Logger

	// replies is this session's share of the fanout. The hub fills it and
	// closes it when the session leaves.
	replies chan *domain.Reply

	// done is closed when the client is gone, so that work being retried on
	// behalf of that client stops instead of being waited out.
	done chan struct{}

	// gone is closed once run has returned, so a shutdown can wait for the
	// session it just stopped.
	gone chan struct{}

	halt sync.Once
}

// run drives the session until the client disconnects or the gateway shuts it
// down, then stops taking new replies, writes out the ones in hand, and closes
// the connection.
func (s *session) run(ctx context.Context) {
	defer close(s.gone)

	// last, so that the replies still in hand are written before the transport
	// goes away.
	defer s.conn.Close()

	leave, joined := s.hub.join(s)
	if !joined {
		// the gateway shut down between accepting this client and serving it.
		s.finish()

		return
	}

	written := make(chan struct{})
	go func() {
		defer close(written)

		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Error("recovered a panic while writing replies", "panic", recovered, "stack", string(debug.Stack()))

				// nothing is left to answer this client, so end the session
				// instead of leaving it taking requests it cannot reply to.
				s.finish()

				if err := s.conn.Close(); err != nil {
					s.logger.Warn("error on closing a client connection", "error", err)
				}
			}
		}()

		s.writeReplies()
	}()

	// deferred, so that a panic in the read loop still unwinds the session:
	// staying in the hub would block writeReplies on a channel nobody closes,
	// stranding the goroutine and everything it holds. Defers run in reverse,
	// so this still happens before the connection is closed.
	defer func() {
		s.finish()
		leave()
		<-written

		// whatever was still being produced for this client has nobody left to
		// receive it. Detached from ctx, which is already done by the time a
		// disconnect unwinds the session.
		s.cancelOutstandingStreams(context.WithoutCancel(ctx))
	}()

	s.readRequests(ctx)
}

// finish stops work being done on this client's behalf, without touching the
// connection.
func (s *session) finish() {
	s.halt.Do(func() {
		close(s.done)
	})
}

// stop disconnects the client from outside its own goroutine. Closing the
// connection is what breaks the read the session is parked on, after which run
// unwinds on its own; waiting on gone is how a caller knows it has.
func (s *session) stop() {
	s.finish()

	if err := s.conn.Close(); err != nil {
		s.logger.Warn("error on closing a client connection", "error", err)
	}

	<-s.gone
}

// readRequests reads client requests until the connection ends or ctx is done.
func (s *session) readRequests(ctx context.Context) {
	for ctx.Err() == nil {
		var request domain.Request

		if err := s.conn.Read(&request); err != nil {
			if errors.Is(err, io.EOF) {
				s.logger.InfoContext(ctx, "client disconnected")
			} else {
				s.logger.ErrorContext(ctx, "error on reading request", "error", err)
			}

			return
		}

		if s.bus.isClosed() {
			s.writeFailure(request.ID, nil, ErrClosed)

			continue
		}

		// a stream named with no subject to carry it to is the client asking
		// for that stream to end.
		if len(request.StreamID) > 0 && len(request.Subject) == 0 {
			s.closeStream(ctx, &request)

			continue
		}

		serverSideID, validationErrors, err := s.dispatcher.dispatch(ctx, &request)

		switch {
		case err != nil:
			if len(serverSideID) > 0 {
				s.forget(serverSideID)
			}

			s.writeFailure(request.ID, nil, err)
		case len(validationErrors) > 0:
			s.writeFailure(request.ID, validationErrors, nil)
		}
	}
}

// writeReplies writes out the replies addressed to this session's requests,
// leaving the rest to the session that registered them.
func (s *session) writeReplies() {
	for reply := range s.replies {
		s.deliver(reply)
	}
}

// deliver writes one reply to the client that is waiting for it. A reply this
// session does not own belongs to another connection and is left alone. A
// lookup that fails and a client whose queue is full are retried on their own
// backoffs; when those run out the request is forgotten, because nothing is
// going to answer it now.
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
			if s.conn.Send(&domain.Reply{RequestID: clientSideID, Kind: reply.Kind, Payload: reply.Payload}) {
				// a chunk is one piece of an answer still being written, so the
				// request stays registered until something ends it.
				if reply.Kind.EndsRequest() {
					s.forget(reply.RequestID)
				}

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
			s.forget(reply.RequestID)

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

// forget releases a request that has been answered or given up on, so its
// client-side id is free to be used again.
func (s *session) forget(serverSideID string) {
	if err := s.registry.DeleteByServerSideID(serverSideID); err != nil && !errors.Is(err, domain.ErrNotExists) {
		s.logger.Error("error on removing a request from the registry", "error", err, "requestID", serverSideID)
	}
}

// writeFailure tells the client its request was rejected. A server-side error
// is reported without its details, which belong in the logs.
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

	if !s.conn.Send(&domain.Reply{RequestID: requestID, Payload: payload}) {
		s.logger.Warn("could not deliver a failure response to the client", "requestID", requestID)
	}
}

// closeStream ends a stream at the client's request: whoever is producing it is
// told to stop, the client is sent the end of the stream it asked to end, and
// the request is released.
func (s *session) closeStream(ctx context.Context, request *domain.Request) {
	validationErrors, err := s.dispatcher.validator.validate(request)
	if err != nil {
		s.writeFailure(request.StreamID, nil, err)

		return
	}

	if len(validationErrors) > 0 {
		s.writeFailure(request.StreamID, validationErrors, nil)

		return
	}

	serverSideID, err := s.registry.GetServerSideID(request.StreamID)
	if err != nil {
		// the stream ended between validating the request and acting on it,
		// which is the outcome the client asked for anyway.
		return
	}

	s.cancelStream(ctx, serverSideID)

	if !s.conn.Send(&domain.Reply{RequestID: request.StreamID, Kind: domain.ReplyEOF}) {
		s.logger.Warn("could not tell the client its stream ended", "streamID", request.StreamID)
	}

	s.forget(serverSideID)
}

// cancelOutstandingStreams tells the producers of everything this client still
// had open that it is gone. A request that was already answered is no longer
// registered, so nothing is cancelled for it; one that never streamed at all
// simply has no producer listening.
func (s *session) cancelOutstandingStreams(ctx context.Context) {
	for _, serverSideID := range s.registry.ServerSideIDs() {
		s.cancelStream(ctx, serverSideID)
	}
}
