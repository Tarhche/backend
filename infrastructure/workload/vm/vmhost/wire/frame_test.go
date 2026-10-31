package wire

import (
	"bytes"
	"errors"
	"io"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFrames(t *testing.T) {
	t.Parallel()

	var connection bytes.Buffer

	frames := []struct {
		t       FrameType
		payload []byte
	}{
		{t: FrameStdin, payload: []byte("echo hello\n")},
		{t: FrameStdin, payload: nil},
		{t: FrameStdout, payload: bytes.Repeat([]byte{'x'}, MaxChunk)},
		{t: FrameResize, payload: ResizePayload(40, 120)},
		{t: FrameExit, payload: ExitPayload(3, nil)},
		{t: FrameWindow, payload: WindowPayload(FrameStdout, Window/2)},
		{t: FrameClose, payload: nil},
	}

	for _, frame := range frames {
		require.NoError(t, WriteFrame(&connection, frame.t, frame.payload))
	}

	reader := NewFrameReader(&connection)

	for _, want := range frames {
		got, payload, err := reader.Next()
		require.NoError(t, err)

		assert.Equal(t, want.t, got)
		assert.Equal(t, len(want.payload), len(payload))
		assert.True(t, bytes.Equal(want.payload, payload))
	}

	_, _, err := reader.Next()
	assert.ErrorIs(t, err, io.EOF, "a connection that ends between frames ends cleanly")
}

func TestFrames_Refused(t *testing.T) {
	t.Parallel()

	t.Run("a frame too long to be this protocol's", func(t *testing.T) {
		t.Parallel()

		_, _, err := NewFrameReader(bytes.NewReader([]byte{2, 0xff, 0xff, 0xff, 0xff})).Next()
		assert.ErrorIs(t, err, ErrProtocol)

		assert.ErrorIs(t, WriteFrame(io.Discard, FrameStdout, make([]byte, maxPayload+1)), ErrProtocol)
	})

	t.Run("a connection that ends inside a frame", func(t *testing.T) {
		t.Parallel()

		_, _, err := NewFrameReader(bytes.NewReader([]byte{2, 0, 0, 0, 9, 'x'})).Next()
		assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
	})

	t.Run("payloads of the wrong length", func(t *testing.T) {
		t.Parallel()

		_, _, err := ParseResize([]byte{1})
		assert.ErrorIs(t, err, ErrProtocol)

		_, _, err = ParseExit([]byte{1})
		assert.ErrorIs(t, err, ErrProtocol)

		_, _, err = ParseWindow([]byte{1})
		assert.ErrorIs(t, err, ErrProtocol)

		_, _, err = ParseWindow(WindowPayload(FrameStdout, 0))
		assert.ErrorIs(t, err, ErrProtocol, "room for nothing is not room")

		_, _, err = ParseWindow(WindowPayload(FrameStdout, Window+1))
		assert.ErrorIs(t, err, ErrProtocol, "room for more than a window was never sent")
	})
}

func TestPayloads(t *testing.T) {
	t.Parallel()

	rows, cols, err := ParseResize(ResizePayload(50, 160))
	require.NoError(t, err)
	assert.Equal(t, []uint{50, 160}, []uint{rows, cols})

	for _, exitCode := range []int{0, 1, 127, -1, math.MaxInt32, math.MinInt32} {
		got, reason, err := ParseExit(ExitPayload(exitCode, nil))
		require.NoError(t, err)

		assert.Equal(t, exitCode, got)
		assert.Empty(t, reason)
	}

	got, reason, err := ParseExit(ExitPayload(0, errors.New("the vm stopped")))
	require.NoError(t, err)
	assert.Equal(t, 0, got)
	assert.Equal(t, "the vm stopped", reason)

	_, reason, err = ParseExit(ExitPayload(0, errors.New(string(bytes.Repeat([]byte{'x'}, 2*maxReason)))))
	require.NoError(t, err)
	assert.Len(t, reason, maxReason, "why is kept short enough for a frame")

	stream, increment, err := ParseWindow(WindowPayload(FrameStdin, 1234))
	require.NoError(t, err)
	assert.Equal(t, FrameStdin, stream)
	assert.Equal(t, 1234, increment)
}

func TestCredit(t *testing.T) {
	t.Parallel()

	t.Run("a window can be sent at once, and no more", func(t *testing.T) {
		t.Parallel()

		credit := NewCredit()

		taken := 0
		for taken < Window {
			n, err := credit.Take(MaxChunk)
			require.NoError(t, err)

			taken += n
		}

		assert.Equal(t, Window, taken)

		took := make(chan int)
		go func() {
			n, _ := credit.Take(MaxChunk)
			took <- n
		}()

		select {
		case <-took:
			t.Fatal("more than a window was sent before any of it was read")
		case <-time.After(50 * time.Millisecond):
		}

		require.NoError(t, credit.Grant(100))
		assert.Equal(t, 100, <-took, "room for 100 bytes is room for 100 bytes")
	})

	t.Run("a stream that ends lets its sender go", func(t *testing.T) {
		t.Parallel()

		credit := NewCredit()
		_, err := credit.Take(Window)
		require.NoError(t, err)

		failed := make(chan error)
		go func() {
			_, err := credit.Take(1)
			failed <- err
		}()

		credit.Fail(io.ErrClosedPipe)
		assert.ErrorIs(t, <-failed, io.ErrClosedPipe)

		_, err = credit.Take(1)
		assert.ErrorIs(t, err, io.ErrClosedPipe)
		assert.NoError(t, credit.Grant(1), "room for a stream that ended is no news")
	})

	t.Run("room for more than a window was never sent", func(t *testing.T) {
		t.Parallel()

		assert.ErrorIs(t, NewCredit().Grant(1), ErrProtocol)
	})
}

func TestInbound(t *testing.T) {
	t.Parallel()

	t.Run("what is read gives the sender its room back, half a window at a time", func(t *testing.T) {
		t.Parallel()

		var (
			lock    sync.Mutex
			granted []int
		)

		inbound := NewInbound(func(n int) {
			lock.Lock()
			defer lock.Unlock()

			granted = append(granted, n)
		})

		require.NoError(t, inbound.Push(bytes.Repeat([]byte{'a'}, Window)))
		assert.ErrorIs(t, inbound.Push([]byte{'b'}), ErrProtocol, "a sender that did not wait for room")

		read := 0
		for read < Window {
			n, err := inbound.Read(make([]byte, MaxChunk))
			require.NoError(t, err)

			read += n
		}

		lock.Lock()
		defer lock.Unlock()

		assert.Equal(t, []int{Window / 2, Window / 2}, granted)
	})

	t.Run("what arrived is read before the stream's end", func(t *testing.T) {
		t.Parallel()

		inbound := NewInbound(nil)

		require.NoError(t, inbound.Push([]byte("last words")))
		inbound.End(io.EOF)
		inbound.End(errors.New("only the first end counts"))

		read, err := io.ReadAll(inbound)
		require.NoError(t, err)
		assert.Equal(t, "last words", string(read))

		require.NoError(t, inbound.Push([]byte("after the end")))
		n, err := inbound.Read(make([]byte, 8))
		assert.Zero(t, n, "what arrives after the end is dropped")
		assert.ErrorIs(t, err, io.EOF)
	})

	t.Run("a stream that fails drops what was not read", func(t *testing.T) {
		t.Parallel()

		inbound := NewInbound(nil)
		require.NoError(t, inbound.Push([]byte("never read")))

		inbound.Fail(io.ErrClosedPipe)

		_, err := inbound.Read(make([]byte, 8))
		assert.ErrorIs(t, err, io.ErrClosedPipe)
	})

	t.Run("a reader waits for what is on its way", func(t *testing.T) {
		t.Parallel()

		inbound := NewInbound(nil)

		read := make(chan string)
		go func() {
			buffer := make([]byte, 8)
			n, _ := inbound.Read(buffer)
			read <- string(buffer[:n])
		}()

		time.Sleep(10 * time.Millisecond)
		require.NoError(t, inbound.Push([]byte("hi")))

		assert.Equal(t, "hi", <-read)
	})
}
