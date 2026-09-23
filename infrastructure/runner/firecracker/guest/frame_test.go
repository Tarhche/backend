package guest

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFrames(t *testing.T) {
	t.Run("a frame reads back as it was written", func(t *testing.T) {
		var stream bytes.Buffer
		require.NoError(t, WriteFrame(&stream, FrameStdout, []byte("hello")))
		require.NoError(t, WriteFrame(&stream, FrameResize, ResizePayload(24, 80)))
		require.NoError(t, WriteFrame(&stream, FrameExit, ExitPayload(-1)))

		frame, err := ReadFrame(&stream)
		require.NoError(t, err)
		assert.Equal(t, Frame{Type: FrameStdout, Payload: []byte("hello")}, frame)

		frame, err = ReadFrame(&stream)
		require.NoError(t, err)
		rows, cols, err := ParseResize(frame.Payload)
		require.NoError(t, err)
		assert.Equal(t, [2]uint16{24, 80}, [2]uint16{rows, cols})

		frame, err = ReadFrame(&stream)
		require.NoError(t, err)
		code, err := ParseExit(frame.Payload)
		require.NoError(t, err)
		assert.Equal(t, -1, code)

		_, err = ReadFrame(&stream)
		assert.ErrorIs(t, err, io.EOF)
	})

	t.Run("a payload larger than a frame is split across as many as it takes", func(t *testing.T) {
		payload := bytes.Repeat([]byte("x"), MaxFramePayload*2+1)

		var stream bytes.Buffer
		require.NoError(t, WriteFrame(&stream, FrameStdin, payload))

		var read []byte
		for stream.Len() > 0 {
			frame, err := ReadFrame(&stream)
			require.NoError(t, err)
			assert.Equal(t, FrameStdin, frame.Type)
			assert.LessOrEqual(t, len(frame.Payload), MaxFramePayload)

			read = append(read, frame.Payload...)
		}

		assert.Equal(t, payload, read)
	})

	t.Run("a frame claiming more than a frame may be is refused before it is read", func(t *testing.T) {
		header := make([]byte, 5)
		header[0] = byte(FrameStdout)
		binary.BigEndian.PutUint32(header[1:], MaxFramePayload+1)

		_, err := ReadFrame(bytes.NewReader(header))
		assert.ErrorIs(t, err, ErrFrameTooLarge)
	})

	t.Run("a frame cut short says so", func(t *testing.T) {
		var stream bytes.Buffer
		require.NoError(t, WriteFrame(&stream, FrameStdout, []byte("hello")))

		_, err := ReadFrame(bytes.NewReader(stream.Bytes()[:stream.Len()-1]))
		assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
	})
}
