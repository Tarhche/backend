package wire

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// An exec session is carried by the connection its request upgraded, in
// frames. A frame is a type byte, the length of its payload as four bytes,
// big-endian, and the payload:
//
//	+------+----------------+-------------------+
//	| type | length (4, BE) | payload (length)  |
//	+------+----------------+-------------------+
//
// The client sends stdin, resize, close and window frames; the vmhost sends
// stdout, stderr, exit, window, and one stdin frame of its own when the
// command's input has closed. Each of the three streams is flow controlled:
// no side sends more of one than the other has said it has room for (Window
// to begin with, and then whatever window frames grant), so neither side ever
// holds more than a window of a stream, and a stream nobody reads holds up
// that stream alone rather than the session.
const (
	// FrameStdin is input for the command. An empty one is the end of the
	// input. From the vmhost, an empty one says the command's input has
	// closed, so whatever is written to it from then on is refused.
	FrameStdin FrameType = 1

	// FrameStdout and FrameStderr are what the command said on either
	// stream. An empty one is the end of that stream.
	FrameStdout FrameType = 2
	FrameStderr FrameType = 3

	// FrameResize is the command's terminal's new size: rows and columns,
	// four bytes each.
	FrameResize FrameType = 4

	// FrameExit is how the command ended: its exit code as four signed bytes,
	// then, when the vmhost could not wait for it, why.
	FrameExit FrameType = 5

	// FrameClose ends the command. A client that hangs up does the same.
	FrameClose FrameType = 6

	// FrameWindow is room for more of a stream: the stream's frame type as one
	// byte, then how many more bytes of it may be sent, as four.
	FrameWindow FrameType = 7
)

const (
	// headerSize is a frame's type and length.
	headerSize = 5

	// MaxChunk is the most of a stream one frame carries.
	MaxChunk = 32 << 10

	// Window is how much of a stream may be sent before the other side has
	// read any of it: the most of it either side ever holds.
	Window = 256 << 10

	// maxPayload is the longest payload a frame may have. Anything longer is
	// a peer that does not speak this.
	maxPayload = 64 << 10

	// maxReason is the most of why the vmhost could not wait for a command
	// that an exit frame carries.
	maxReason = 4 << 10
)

// ErrProtocol is a peer that sent something this protocol does not have.
var ErrProtocol = errors.New("the exec session's peer does not speak " + ExecProtocol)

// FrameType says what a frame carries.
type FrameType byte

func (t FrameType) String() string {
	switch t {
	case FrameStdin:
		return "stdin"
	case FrameStdout:
		return "stdout"
	case FrameStderr:
		return "stderr"
	case FrameResize:
		return "resize"
	case FrameExit:
		return "exit"
	case FrameClose:
		return "close"
	case FrameWindow:
		return "window"
	default:
		return fmt.Sprintf("frame(%d)", byte(t))
	}
}

// WriteFrame writes one frame in one write, so frames written by several
// writers under one lock never interleave.
func WriteFrame(w io.Writer, t FrameType, payload []byte) error {
	if len(payload) > maxPayload {
		return fmt.Errorf("%w: a %s frame of %d bytes", ErrProtocol, t, len(payload))
	}

	frame := make([]byte, headerSize+len(payload))
	frame[0] = byte(t)
	binary.BigEndian.PutUint32(frame[1:headerSize], uint32(len(payload)))
	copy(frame[headerSize:], payload)

	_, err := w.Write(frame)

	return err
}

// FrameReader reads the frames a connection carries, one at a time.
type FrameReader struct {
	reader io.Reader
	header [headerSize]byte
	buffer []byte
}

func NewFrameReader(r io.Reader) *FrameReader {
	return &FrameReader{reader: r, buffer: make([]byte, maxPayload)}
}

// Next is the next frame. Its payload is good until the next call, so a
// payload that is kept has to be copied.
func (r *FrameReader) Next() (FrameType, []byte, error) {
	if _, err := io.ReadFull(r.reader, r.header[:]); err != nil {
		return 0, nil, err
	}

	length := binary.BigEndian.Uint32(r.header[1:])
	if length > maxPayload {
		return 0, nil, fmt.Errorf("%w: a frame of %d bytes", ErrProtocol, length)
	}

	payload := r.buffer[:length]
	if _, err := io.ReadFull(r.reader, payload); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}

		return 0, nil, err
	}

	return FrameType(r.header[0]), payload, nil
}

// ResizePayload is a resize frame's payload.
func ResizePayload(rows uint, cols uint) []byte {
	payload := make([]byte, 8)
	binary.BigEndian.PutUint32(payload[:4], clamp(rows))
	binary.BigEndian.PutUint32(payload[4:], clamp(cols))

	return payload
}

// ParseResize reads a resize frame's payload.
func ParseResize(payload []byte) (rows uint, cols uint, err error) {
	if len(payload) != 8 {
		return 0, 0, fmt.Errorf("%w: a resize of %d bytes", ErrProtocol, len(payload))
	}

	return uint(binary.BigEndian.Uint32(payload[:4])), uint(binary.BigEndian.Uint32(payload[4:])), nil
}

// ExitPayload is an exit frame's payload: the command's exit code, and why it
// could not be waited for when it could not.
func ExitPayload(exitCode int, waitErr error) []byte {
	var reason string
	if waitErr != nil {
		reason = waitErr.Error()
		if len(reason) > maxReason {
			reason = reason[:maxReason]
		}
	}

	payload := make([]byte, 4+len(reason))
	binary.BigEndian.PutUint32(payload[:4], uint32(int32(max(min(exitCode, math.MaxInt32), math.MinInt32))))
	copy(payload[4:], reason)

	return payload
}

// ParseExit reads an exit frame's payload. A reason is the vmhost not having
// been able to wait for the command.
func ParseExit(payload []byte) (exitCode int, reason string, err error) {
	if len(payload) < 4 {
		return 0, "", fmt.Errorf("%w: an exit of %d bytes", ErrProtocol, len(payload))
	}

	return int(int32(binary.BigEndian.Uint32(payload[:4]))), string(payload[4:]), nil
}

// WindowPayload is a window frame's payload: room for increment more bytes
// of stream.
func WindowPayload(stream FrameType, increment int) []byte {
	payload := make([]byte, 5)
	payload[0] = byte(stream)
	binary.BigEndian.PutUint32(payload[1:], uint32(increment))

	return payload
}

// ParseWindow reads a window frame's payload.
func ParseWindow(payload []byte) (stream FrameType, increment int, err error) {
	if len(payload) != 5 {
		return 0, 0, fmt.Errorf("%w: a window of %d bytes", ErrProtocol, len(payload))
	}

	increment = int(binary.BigEndian.Uint32(payload[1:]))
	if increment == 0 || increment > Window {
		return 0, 0, fmt.Errorf("%w: a window of %d more bytes", ErrProtocol, increment)
	}

	return FrameType(payload[0]), increment, nil
}

// clamp is a terminal dimension as four bytes hold it.
func clamp(n uint) uint32 {
	return uint32(min(n, math.MaxUint32))
}
