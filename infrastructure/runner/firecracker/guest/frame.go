package guest

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// A command's input, output, window size and end all travel down the one
// connection it was started on, as frames: a type, a length, and that many
// bytes.
//
//	type(1) | length(4, big endian) | payload(length)
type FrameType byte

const (
	// FrameStdin is input for the command.
	FrameStdin FrameType = 0

	// FrameStdout and FrameStderr are its output. With a terminal there is
	// only one stream, and it is stdout.
	FrameStdout FrameType = 1
	FrameStderr FrameType = 2

	// FrameResize is the size of its terminal: rows, then columns, each two
	// bytes.
	FrameResize FrameType = 3

	// FrameExit is the last frame of a command, carrying what it returned in
	// four bytes.
	FrameExit FrameType = 4

	// FrameCloseStdin says there is no more input.
	FrameCloseStdin FrameType = 5
)

// MaxFramePayload bounds a frame, so a peer claiming an enormous one cannot
// make the other end allocate it.
const MaxFramePayload = 64 << 10

// ErrFrameTooLarge is a frame claiming more than MaxFramePayload.
var ErrFrameTooLarge = errors.New("frame is larger than a frame may be")

const frameHeaderSize = 5

// Frame is one frame.
type Frame struct {
	Type    FrameType
	Payload []byte
}

// WriteFrame writes one frame. A payload larger than a frame may be is split
// across as many as it takes.
func WriteFrame(w io.Writer, t FrameType, payload []byte) error {
	for {
		chunk := payload
		if len(chunk) > MaxFramePayload {
			chunk = chunk[:MaxFramePayload]
		}

		frame := make([]byte, frameHeaderSize+len(chunk))
		frame[0] = byte(t)
		binary.BigEndian.PutUint32(frame[1:frameHeaderSize], uint32(len(chunk)))
		copy(frame[frameHeaderSize:], chunk)

		if _, err := w.Write(frame); err != nil {
			return err
		}

		payload = payload[len(chunk):]
		if len(payload) == 0 {
			return nil
		}
	}
}

// ReadFrame reads one frame.
func ReadFrame(r io.Reader) (Frame, error) {
	var header [frameHeaderSize]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return Frame{}, err
	}

	length := binary.BigEndian.Uint32(header[1:])
	if length > MaxFramePayload {
		return Frame{}, fmt.Errorf("%w: %d bytes", ErrFrameTooLarge, length)
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}

		return Frame{}, err
	}

	return Frame{Type: FrameType(header[0]), Payload: payload}, nil
}

// ResizePayload is the payload of a FrameResize.
func ResizePayload(rows uint16, cols uint16) []byte {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint16(payload[0:2], rows)
	binary.BigEndian.PutUint16(payload[2:4], cols)

	return payload
}

// ParseResize reads a FrameResize's payload.
func ParseResize(payload []byte) (uint16, uint16, error) {
	if len(payload) != 4 {
		return 0, 0, fmt.Errorf("a resize is four bytes, not %d", len(payload))
	}

	return binary.BigEndian.Uint16(payload[0:2]), binary.BigEndian.Uint16(payload[2:4]), nil
}

// ExitPayload is the payload of a FrameExit.
func ExitPayload(code int) []byte {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload, uint32(int32(code)))

	return payload
}

// ParseExit reads a FrameExit's payload.
func ParseExit(payload []byte) (int, error) {
	if len(payload) != 4 {
		return 0, fmt.Errorf("an exit is four bytes, not %d", len(payload))
	}

	return int(int32(binary.BigEndian.Uint32(payload))), nil
}
