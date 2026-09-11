package tunnel

import (
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

// StreamProxy joins two connections and copies between them until both are
// done.
//
// It is the whole of the data path, and it is deliberately ignorant: it reads
// bytes and it writes them. Nothing here knows whether what is going through is
// ssh, a database protocol or a file — which is what lets the tunnel carry
// anything at all.
//
// # Backpressure
//
// There is no queue here. A copy is a read followed by the write of exactly
// what was read, so a direction moves only as fast as its slower end, and a
// stalled write stalls the read behind it:
//
//	client ── TCP ──> ingress ── smux ──> worker ── TCP ──> target
//
// A slow target stops reading its TCP connection, its socket buffer fills, the
// worker's write blocks, so the worker stops reading the stream, so its receive
// window closes. smux then refuses the ingress's writes, the ingress stops
// reading the client, the client's socket buffer fills, and the client's own
// writes block. Every step is a bounded buffer that was already there. The same
// happens in reverse for a slow client.
//
// What is held in flight per stream is therefore its smux window plus the two
// copy buffers, and nothing else — which is why the windows are a memory budget
// in config.go rather than a performance dial.
type StreamProxy struct {
	// BufferSize is what one direction copies through. Zero uses the package
	// default.
	BufferSize int

	// HalfCloseLinger is how long the second direction to finish waits before
	// saying so, when the first one ended because the far end had already said
	// it. Zero uses the package default.
	//
	// It works around a data loss in smux v1.5.57: completing a half-close —
	// both ends having closed their write half — tears the stream down through
	// streamClosed, and recycleTokens throws away whatever had arrived and not
	// yet been read. The request-and-answer shape that half-close exists for is
	// exactly the shape that loses its answer, because the answer and the FIN
	// behind it arrive together.
	//
	// So the side closing second gives the side that already finished a moment
	// to read what it was just sent. It costs nothing on any connection that
	// did not half-close, because there is no teardown to race.
	HalfCloseLinger time.Duration
}

// Copy runs both directions and returns when both have finished, reporting how
// much went each way. sent is what came from left and reached right.
//
// A direction that ends closes only the writing half of the other side, so the
// far end learns that nothing more is coming while it is still free to answer.
// That is what makes a half-closed connection stay half-closed across the
// tunnel rather than becoming a closed one — something a protocol that waits
// for the answer after it has finished asking depends on.
func (p StreamProxy) Copy(left net.Conn, right net.Conn) (sent int64, received int64, err error) {
	size := p.BufferSize
	if size <= 0 {
		size = copyBufferSize
	}

	linger := p.HalfCloseLinger
	if linger <= 0 {
		linger = defaultHalfCloseLinger
	}

	var (
		wait      sync.WaitGroup
		leftErr   error
		rightErr  error
		forward   int64
		backwards int64

		// finished is closed by whichever direction ends first, so the other
		// one knows the far end is already waiting on it.
		finished  sync.Once
		firstDone = make(chan struct{})
	)

	wait.Add(2)

	go func() {
		defer wait.Done()

		forward, leftErr = copyOneWay(right, left, size, firstDone, linger)

		finished.Do(func() { close(firstDone) })
	}()

	go func() {
		defer wait.Done()

		backwards, rightErr = copyOneWay(left, right, size, firstDone, linger)

		finished.Do(func() { close(firstDone) })
	}()

	wait.Wait()

	// both directions are finished, so neither connection is anybody's any
	// more.
	left.Close()
	right.Close()

	return forward, backwards, errors.Join(leftErr, rightErr)
}

// copyOneWay moves bytes from src to dst and then tells dst that there will be
// no more.
func copyOneWay(dst net.Conn, src net.Conn, size int, first <-chan struct{}, linger time.Duration) (int64, error) {
	buffer := make([]byte, size)

	n, err := io.CopyBuffer(writerOnly{dst}, readerOnly{src}, buffer)

	// the other direction is already over, so saying this one is too completes
	// the half-close — and completing it is what can throw away what was just
	// written. Give the far end the moment it needs to read it.
	select {
	case <-first:
		if n > 0 {
			time.Sleep(linger)
		}
	default:
	}

	// whatever happened, the direction is over: closing the writing half is
	// what turns "I have stopped" into something the far end can see.
	closeWrite(dst)

	if err == nil || errors.Is(err, io.EOF) || isClosed(err) {
		return n, nil
	}

	return n, err
}

// closeWrite ends one direction where that is possible, and settles for ending
// both where it is not.
func closeWrite(conn net.Conn) {
	if closer, ok := conn.(halfCloser); ok {
		_ = closer.CloseWrite()

		return
	}

	_ = conn.Close()
}

// isClosed reports the ordinary end of a connection, which is not a failure to
// be reported as one.
func isClosed(err error) bool {
	return errors.Is(err, net.ErrClosed) || errors.Is(err, io.ErrClosedPipe)
}

// readerOnly and writerOnly hide ReadFrom and WriteTo so that io.CopyBuffer
// uses the buffer it was given. Left exposed, smux's own WriteTo would be
// chosen and the copy would allocate a buffer of its own, which is the one
// place the bound above could quietly stop holding.
type readerOnly struct{ io.Reader }

type writerOnly struct{ io.Writer }
