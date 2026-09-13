// Package tunnel carries arbitrary TCP to an agent over connections the agent
// opened.
//
// An agent sits behind NAT with no inbound port and no address anyone could
// dial. It opens a few persistent TCP connections outwards to the hub and
// keeps them; the hub multiplexes every client connection it receives onto
// them as independent streams. Nothing ever dials an agent, so being connected
// and being reachable are the same fact.
//
// # The data path
//
//	client TCP ─┐
//	client TCP ─┼──> hub ──> smux stream ──> agent ──> target TCP
//	client TCP ─┘                    │
//	                                 └─ over one of the agent's own
//	                                    persistent TCP connections
//
// Nothing else is on that path. There is no broker, no queue and no store: the
// bytes go from one socket to another through smux's flow control and nothing
// buffers them but the windows either end agreed on.
//
// # Components
//
//	Hub           takes the agents' connections, registers them, and opens
//	              streams on them. Knows nothing about what a stream carries.
//	Agent         the far end: a pool of connections to each hub, and the
//	              thing that accepts streams and joins them to targets.
//	Registry      which agents are connected and with what. The only shared
//	              state, and behind an interface so it can become a shared one.
//	Session       one smux session over one of an agent's TCP connections,
//	              with its own capacity and its own counters.
//	Router        picks an agent for a client that did not name one.
//	Targets       what an agent will connect a stream to, and what it will not.
//	Authenticator what the hub will take a connection from.
//	Forwarder     the ports arbitrary TCP arrives on, each carried to an agent.
//	StreamProxy   the byte pipe, both directions, with half-close.
//
// # Which end is the smux client
//
// The agent dials, but the hub is what opens streams — so the hub is the smux
// *client* and the agent the smux *server*, on a connection the agent made. smux gives the client odd stream ids and the server even ones,
// so putting them the other way round makes both ends hand out ids the other
// does not expect. The roles follow who opens streams, not who opened the
// socket.
//
// # Failure isolation
//
// An agent holds several sessions on purpose. A session is one TCP connection,
// and one TCP connection is one thing that can break: when it does, the streams
// on it fail the way a TCP connection fails, and the sessions beside it carry
// on. Nothing is migrated — a stream whose session died is gone, and only new
// streams are routed elsewhere. Moving a live connection between sessions would
// mean replaying bytes neither end can replay.
package tunnel
