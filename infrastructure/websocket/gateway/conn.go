// Package gateway carries a client's requests to the message broker and the
// replies back, over whatever transport the client is connected on.
//
// A client sends requests on subjects the gateway consumes. Each request is
// produced onto the broker so that exactly one replica handles it, and its
// reply is published to every replica so that it reaches whichever one is
// holding the client:
//
//	client -> session -> dispatcher -> broker (produced once, for one replica)
//	broker (published to every replica) -> replyBus -> hub -> session -> client
//
// None of that is transport work. The gateway reads domain.Requests and writes
// domain.Replies through a Conn, so a websocket, a raw TCP stream or a protocol
// of your own becomes a client of this package by implementing Conn and handing
// its connections to Sessions.
package gateway

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
)

// Conn is one client's transport, as the gateway sees it: whole requests in,
// whole replies out. Framing, encoding, keepalive and backpressure are the
// transport's business.
//
// Two goroutines use a Conn at once, so Read must be safe alongside Send and
// Close. Send must also be safe against itself: the loop reading requests
// answers the ones it rejects while the writer is delivering replies. Read is
// only ever called from one goroutine.
type Conn interface {
	// Read blocks until the next request arrives and decodes it. It reports
	// io.EOF when the conversation ended — the client hung up, or this side
	// closed the connection — which is how the gateway tells an ordinary
	// goodbye from a broken transport.
	Read(request *domain.Request) error

	// Send hands a reply to the client and reports whether the transport took
	// it. It must not block on a client that is not reading: a reply that
	// cannot be queued is refused, so the gateway can retry it or give up
	// rather than stall the client's other replies behind it.
	Send(reply *domain.Reply) bool

	// Close disconnects the client and releases the transport. It must be safe
	// to call more than once and from a goroutine other than the one reading,
	// because it is what breaks a blocked Read when the gateway shuts down.
	Close() error
}

// Sessions is what a transport hands its connections to. *Gateway implements
// it; a transport depends on this interface instead of the concrete gateway, so
// it can be driven by a double in tests and knows nothing about how a request
// is validated, dispatched, or matched to its reply.
type Sessions interface {
	// Accept reports whether a new client can still be served. It returns
	// ErrClosed once the gateway has shut down, so that a transport can refuse
	// a client before committing to a connection it could never answer on.
	Accept() error

	// Serve drives one client's conversation until that client goes away or the
	// gateway shuts down. It closes the connection before returning.
	Serve(ctx context.Context, conn Conn)
}
