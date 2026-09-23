package agent

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/guest"
)

const (
	// logLineLimit caps a single line, so a task writing one enormous line
	// without a newline cannot grow what holds it without bound. Once the
	// limit is reached, what has been read is kept as a line of its own.
	logLineLimit = 64 << 10

	// defaultLogBytes is how much of a task's output a machine keeps for the
	// orchestrator to collect. It only has to cover the time nobody is
	// following: the orchestrator keeps everything it has read.
	defaultLogBytes = 4 << 20
)

// ring keeps the most recent of a task's output, numbered, for whoever reads
// it to take up where it left off.
type ring struct {
	lock  sync.Mutex
	limit int

	lines []guest.LogLine
	bytes int

	// next is the number the next line gets. Numbers start at one, so
	// "after zero" is everything.
	next uint64

	// changed is closed, and replaced, whenever a line is added.
	changed chan struct{}
}

func newRing(limit int) *ring {
	return &ring{limit: limit, next: 1, changed: make(chan struct{})}
}

// add keeps one line, letting go of the oldest ones if it has to.
func (r *ring) add(stream string, content string, at time.Time) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.lines = append(r.lines, guest.LogLine{Seq: r.next, Stream: stream, At: at.UTC(), Content: content})
	r.bytes += len(content)
	r.next++

	dropped := 0
	for r.bytes > r.limit && dropped < len(r.lines)-1 {
		r.bytes -= len(r.lines[dropped].Content)
		dropped++
	}

	if dropped > 0 {
		r.lines = append(r.lines[:0:0], r.lines[dropped:]...)
	}

	close(r.changed)
	r.changed = make(chan struct{})
}

// after is every line kept that came after the one numbered seq, and what is
// closed once there is another.
func (r *ring) after(seq uint64) ([]guest.LogLine, <-chan struct{}) {
	r.lock.Lock()
	defer r.lock.Unlock()

	start := len(r.lines)
	for i, line := range r.lines {
		if line.Seq > seq {
			start = i

			break
		}
	}

	return append([]guest.LogLine(nil), r.lines[start:]...), r.changed
}

// pump reads a stream line by line into the ring until it ends.
func (r *ring) pump(stream string, reader io.Reader) {
	buffered := bufio.NewReaderSize(reader, logLineLimit)

	for {
		line, err := buffered.ReadSlice('\n')
		if len(line) > 0 {
			r.add(stream, strings.TrimRight(string(line), "\r\n"), time.Now())
		}

		// a line longer than the buffer is kept in pieces rather than lost.
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}

		if err != nil {
			return
		}
	}
}
