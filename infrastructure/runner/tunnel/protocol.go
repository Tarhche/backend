package tunnel

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

// The protocol, in full.
//
// Two exchanges, both a single json object followed by a newline, and nothing
// else is ever spoken: after each one the connection or the stream is a plain
// byte pipe, which is what makes this carry ssh or anything else without
// knowing what it is.
//
//	registration, once per TCP connection, before smux starts
//	    worker  -> ingress   {"version":1,"worker":"…","token":"…"}
//	    ingress -> worker    {"ok":true,"session":"…"}
//	    … from here the connection belongs to smux
//
//	opening, once per stream, before any payload
//	    ingress -> worker    {"service":"…"}  or  {"host":"…","port":22}
//	    worker  -> ingress   {"ok":true}
//	    … from here the stream is the client's bytes and the target's
//
// The opening is acknowledged so that a target which could not be reached is
// distinguishable from one that accepted and said nothing, which at layer four
// look identical: both are a closed stream. It costs one round trip on a
// connection that already exists.
const ProtocolVersion = 1

const (
	// maxFrameBytes bounds what is read before either end has proved it speaks
	// this protocol at all.
	maxFrameBytes = 4 << 10
)

var (
	// ErrProtocol is a peer that did not speak this protocol.
	ErrProtocol = errors.New("tunnel: protocol error")

	// ErrUnsupportedVersion is a peer speaking a version this does not.
	ErrUnsupportedVersion = errors.New("tunnel: unsupported protocol version")

	// ErrRejected is a connection or a stream the far end would not take. It
	// wraps whatever the far end said was wrong with it.
	ErrRejected = errors.New("tunnel: rejected")
)

// registration is what a worker says on a new connection: which worker it is,
// and what entitles it to say so.
type registration struct {
	Version int    `json:"version"`
	Worker  string `json:"worker"`
	Token   string `json:"token,omitempty"`
}

// registered is the ingress's answer. A worker that is not welcome is told why
// before the connection closes, so a misconfigured one says so in its log
// rather than looping in silence.
type registered struct {
	OK      bool   `json:"ok"`
	Session string `json:"session,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Target is what a stream is to be connected to.
//
// A service is a name the worker resolves for itself, which is how one worker
// comes to offer several things without the ingress knowing what any of them
// are. A host and port is the same request made explicitly, and is checked
// against what the worker will allow.
type Target struct {
	Service string `json:"service,omitempty"`
	Host    string `json:"host,omitempty"`
	Port    uint16 `json:"port,omitempty"`
}

func (t Target) String() string {
	if len(t.Service) > 0 {
		return t.Service
	}

	return net.JoinHostPort(t.Host, fmt.Sprint(t.Port))
}

// Named reports whether the target asks for a service by name rather than for
// an address.
func (t Target) Named() bool {
	return len(t.Service) > 0
}

// Valid reports whether the target asks for anything at all.
func (t Target) Valid() bool {
	return t.Named() || (len(t.Host) > 0 && t.Port > 0)
}

// opened is the worker's answer to a stream: whether it reached the target.
type opened struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// writeFrame writes one json message followed by a newline.
func writeFrame(conn net.Conn, message any, deadline time.Time) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}

	if err := conn.SetWriteDeadline(deadline); err != nil {
		return err
	}
	defer conn.SetWriteDeadline(time.Time{})

	_, err = conn.Write(append(payload, '\n'))

	return err
}

// readFrame reads one newline-terminated json message.
//
// It reads through the given reader rather than the connection so that nothing
// read past the newline is lost: what follows on these connections belongs to
// smux or to the client, and dropping a byte of it would be silent corruption.
// Callers check Buffered afterwards for exactly that reason.
func readFrame(conn net.Conn, reader *bufio.Reader, message any, deadline time.Time) error {
	if err := conn.SetReadDeadline(deadline); err != nil {
		return err
	}
	defer conn.SetReadDeadline(time.Time{})

	line, err := reader.ReadSlice('\n')
	if err != nil {
		if errors.Is(err, bufio.ErrBufferFull) {
			return errors.Join(ErrProtocol, io.ErrShortBuffer)
		}

		return err
	}

	if err := json.Unmarshal(line, message); err != nil {
		return errors.Join(ErrProtocol, err)
	}

	return nil
}

// rejection turns what the far end said into an error that says both that it
// refused and why.
func rejection(reason string) error {
	if len(reason) == 0 {
		reason = "no reason given"
	}

	return errors.Join(ErrRejected, errors.New(reason))
}

// halfCloser is anything that can end one direction and leave the other open.
// Both a TCP connection and a smux stream can, which is what lets a half-close
// travel the whole way through the tunnel instead of stopping at it.
type halfCloser interface {
	CloseWrite() error
}

// writeStreamFrame writes one frame onto a stream, before the payload.
func writeStreamFrame(stream net.Conn, message any, deadline time.Time) error {
	return writeFrame(stream, message, deadline)
}

// readStreamFrame reads one frame from a stream and returns whatever was read
// past it.
//
// The payload begins immediately after the newline, so anything the buffered
// read took beyond it belongs to the client or the target and has to be handed
// back rather than dropped. In practice the far end writes the frame and then
// waits, so there is usually nothing; correctness cannot rest on usually.
func readStreamFrame(stream net.Conn, message any, deadline time.Time) ([]byte, error) {
	if err := stream.SetReadDeadline(deadline); err != nil {
		return nil, err
	}
	defer stream.SetReadDeadline(time.Time{})

	reader := bufio.NewReaderSize(stream, maxFrameBytes)

	line, err := reader.ReadSlice('\n')
	if err != nil {
		if errors.Is(err, bufio.ErrBufferFull) {
			return nil, errors.Join(ErrProtocol, io.ErrShortBuffer)
		}

		return nil, err
	}

	if err := json.Unmarshal(line, message); err != nil {
		return nil, errors.Join(ErrProtocol, err)
	}

	leftover := make([]byte, reader.Buffered())
	if _, err := io.ReadFull(reader, leftover); err != nil {
		return nil, err
	}

	return leftover, nil
}

// withPrefix puts bytes already read back in front of a connection.
func withPrefix(conn net.Conn, prefix []byte) net.Conn {
	if len(prefix) == 0 {
		return conn
	}

	return &prefixedConn{Conn: conn, reader: io.MultiReader(bytes.NewReader(prefix), conn)}
}

// prefixedConn is a connection whose first bytes were read before it was handed
// over.
type prefixedConn struct {
	net.Conn

	reader io.Reader
}

var (
	_ net.Conn   = &prefixedConn{}
	_ halfCloser = &prefixedConn{}
)

func (c *prefixedConn) Read(b []byte) (int, error) { return c.reader.Read(b) }

func (c *prefixedConn) CloseWrite() error {
	if closer, ok := c.Conn.(halfCloser); ok {
		return closer.CloseWrite()
	}

	return nil
}
