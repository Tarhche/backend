// Package terminal serves a terminal over a websocket request.
//
// Opening one is asking a question whose answer never ends: the reply to the
// request is the command's output, chunk by chunk, until either the command
// finishes or the person watching goes away. What they type comes back on a
// second subject naming that same stream, so a terminal is one conversation
// rather than a connection of its own.
//
// Who may open one, and on which container, is not decided here — the dashboard
// asks about permissions and the code runner asks whether the container is one
// of its own. This is what both of them do afterwards.
package terminal

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"

	"github.com/khanzadimahdi/testproject/domain"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/gateway"
)

// chunkSize is how much of a command's output is carried in one reply.
const chunkSize = 4 << 10

// the kinds of input a terminal takes.
const (
	InputKeys   = ""
	InputResize = "resize"
)

// Input is what a client sends to a terminal it already has open: the keys
// somebody pressed, or the size of the window they are pressing them in.
type Input struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data []byte `json:"data"`
	Rows uint   `json:"rows"`
	Cols uint   `json:"cols"`
}

// Sessions are the terminals this replica has open, so that what a client types
// reaches the one it typed into.
//
// A terminal only exists on the replica that opened it, and input is produced
// onto the broker, so exactly one replica is handed each keystroke. With more
// than one replica serving the site, that need not be the one holding the
// terminal — so a terminal is a single-replica feature until input travels the
// way replies do, published to every replica rather than produced to one.
type Sessions struct {
	replyer domain.Replyer

	// streams is what tells this replica that a client has stopped listening,
	// so the terminal it was holding is closed rather than left running.
	streams *gateway.Streams

	lock sync.RWMutex
	open map[string]runnerManager.Attachment

	logger *slog.Logger
}

func NewSessions(replyer domain.Replyer, streams *gateway.Streams, logger *slog.Logger) *Sessions {
	return &Sessions{
		replyer: replyer,
		streams: streams,
		open:    make(map[string]runnerManager.Attachment),
		logger:  logger,
	}
}

// Serve pumps a terminal's output back to its client until one end stops.
func (s *Sessions) Serve(ctx context.Context, requestID string, attachment runnerManager.Attachment) {
	// detached from the message that opened it: a terminal lives as long as
	// the person using it, not as long as the request that asked for one.
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))

	s.add(requestID, attachment)
	s.streams.Add(requestID, cancel)

	go func() {
		defer func() {
			s.streams.Remove(requestID)
			s.remove(requestID)
			_ = attachment.Close()
			cancel()

			s.end(ctx, requestID)
		}()

		// closing the attachment is what releases a read parked on it, so a
		// cancelled context has to reach it that way.
		stop := context.AfterFunc(ctx, func() { _ = attachment.Close() })
		defer stop()

		buffer := make([]byte, chunkSize)

		for {
			n, err := attachment.Read(buffer)

			if n > 0 {
				// the bytes are the payload: a terminal's output is not JSON,
				// and a reply carries it to the client as it stands.
				chunk := make([]byte, n)
				copy(chunk, buffer[:n])

				if err := s.replyer.Reply(ctx, &domain.Reply{
					RequestID: requestID,
					Kind:      domain.ReplyChunk,
					Payload:   chunk,
				}); err != nil {
					s.logger.ErrorContext(ctx, "error on writing terminal output", "error", err)

					return
				}
			}

			if err != nil {
				if !errors.Is(err, io.EOF) {
					s.logger.WarnContext(ctx, "a terminal ended", "error", err)
				}

				return
			}
		}
	}()
}

// Write carries what somebody typed, or the size of their window, to the
// terminal they have open.
func (s *Sessions) Write(ctx context.Context, data []byte) error {
	var input Input
	if err := json.Unmarshal(data, &input); err != nil {
		return nil
	}

	attachment, ok := s.get(input.ID)
	if !ok {
		// the terminal has ended, or was opened on another replica. Either way
		// there is nothing here to write to, and redelivering would not find
		// it.
		s.logger.WarnContext(ctx, "input for a terminal this replica does not hold", "streamID", input.ID)

		return nil
	}

	switch input.Type {
	case InputResize:
		if input.Rows == 0 || input.Cols == 0 {
			return nil
		}

		return attachment.Resize(ctx, input.Rows, input.Cols)

	case InputKeys:
		if len(input.Data) == 0 {
			return nil
		}

		_, err := attachment.Write(input.Data)

		return err

	default:
		return nil
	}
}

// Fail answers a request that never opened a terminal, and ends it, so the
// client is not left waiting on a stream that will never start.
func (s *Sessions) Fail(ctx context.Context, requestID string, validationErrors domain.ValidationErrors) error {
	payload, err := json.Marshal(map[string]any{"errors": validationErrors})
	if err != nil {
		return err
	}

	return s.replyer.Reply(ctx, &domain.Reply{
		RequestID: requestID,
		Kind:      domain.ReplyEOF,
		Payload:   payload,
	})
}

// end tells the client that its terminal is over.
func (s *Sessions) end(ctx context.Context, requestID string) {
	if err := s.replyer.Reply(ctx, &domain.Reply{RequestID: requestID, Kind: domain.ReplyEOF}); err != nil {
		s.logger.ErrorContext(ctx, "error on ending a terminal", "error", err)
	}
}

func (s *Sessions) add(requestID string, attachment runnerManager.Attachment) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.open[requestID] = attachment
}

func (s *Sessions) get(requestID string) (runnerManager.Attachment, bool) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	attachment, ok := s.open[requestID]

	return attachment, ok
}

func (s *Sessions) remove(requestID string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.open, requestID)
}
