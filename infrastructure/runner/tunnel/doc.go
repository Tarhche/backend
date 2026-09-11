// Package tunnel carries arbitrary TCP to a worker over connections the worker
// opened.
//
// A worker sits behind NAT with no inbound port and no address anyone could
// dial. It opens a few persistent TCP connections outwards to the ingress and
// keeps them; the ingress multiplexes every client connection it receives onto
// them as independent streams. Nothing ever dials a worker, so being connected
// and being reachable are the same fact.
//
// # The data path
//
//	client TCP ─┐
//	client TCP ─┼──> ingress ──> smux stream ──> worker ──> target TCP
//	client TCP ─┘                    │
//	                                 └─ over one of the worker's own
//	                                    persistent TCP connections
//
// Nothing else is on that path. There is no broker, no queue and no store: the
// bytes go from one socket to another through smux's flow control and nothing
// buffers them but the windows either end agreed on.
//
// # Components
//
//	Ingress      takes the workers' connections, registers them, and opens
//	             streams on them. Knows nothing about what a stream carries.
//	Registry     which workers are connected and with what. The only shared
//	             state, and behind an interface so it can become a shared one.
//	Session      one smux session over one of a worker's TCP connections,
//	             with its own capacity and its own counters.
//	Router       picks a worker for a client that did not name one.
//	Worker       the far end: a pool of connections to the ingresses, and the
//	             thing that accepts streams and connects them to targets.
//	Targets      what a worker will connect a stream to, and what it will not.
//	Authenticator what the ingress will take a connection from.
//	StreamProxy  the byte pipe, both directions, with half-close.
//
// # Which end is the smux client
//
// The worker dials, but the ingress is what opens streams — so the ingress is
// the smux *client* and the worker the smux *server*, on a connection the
// worker made. smux gives the client odd stream ids and the server even ones,
// so putting them the other way round makes both ends hand out ids the other
// does not expect. The roles follow who opens streams, not who opened the
// socket.
//
// # Failure isolation
//
// A worker holds several sessions on purpose. A session is one TCP connection,
// and one TCP connection is one thing that can break: when it does, the streams
// on it fail the way a TCP connection fails, and the sessions beside it carry
// on. Nothing is migrated — a stream whose session died is gone, and only new
// streams are routed elsewhere. Moving a live connection between sessions would
// mean replaying bytes neither end can replay.
package tunnel
