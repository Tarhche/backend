package agent

import (
	"bufio"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
)

const (
	// logLineLimit caps a single line, so a task writing one enormous line
	// without a newline cannot grow what holds it without bound. Once the
	// limit is reached, what has been read is kept as a line of its own.
	logLineLimit = 64 << 10

	// defaultLogBytes is how much of a task's output a machine keeps for
	// vmhost to collect. It only has to cover the time nobody is following:
	// vmhost keeps everything it has read.
	defaultLogBytes = 4 << 20

	// lineOverhead is what keeping a line costs besides its content: the
	// line itself, its number, its stream and its time. It is counted with
	// the content, so a task printing a great many short lines is held to
	// the same memory as one printing a few long ones, rather than to many
	// times what the ring is meant to hold.
	lineOverhead = 64
)

// ring keeps the most recent of a task's output, numbered, for whoever reads
// it to take up where it left off.
//
// Lines are numbered one after another and only ever let go of from the
// oldest end, so the numbers of the lines kept are consecutive: where a line
// is in the ring follows from its number, without looking for it.
type ring struct {
	lock  sync.Mutex
	limit int

	// lines are the lines kept, oldest first, from head on: the ones before
	// head are gone, and their place is given back now and then rather than
	// every time, so a full ring takes a line in without moving all the
	// others to make room for it.
	lines []guest.LogLine
	head  int
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

// cost is what keeping a line of content counts against the ring's limit.
func cost(content string) int {
	return lineOverhead + len(content)
}

// add keeps one line, letting go of the oldest ones if it has to. The line
// just added is always kept, however large: a line larger than the whole
// ring is still a line the task wrote.
func (r *ring) add(stream string, content string, at time.Time) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.lines = append(r.lines, guest.LogLine{Seq: r.next, Stream: stream, At: at.UTC(), Content: content})
	r.bytes += cost(content)
	r.next++

	for r.bytes > r.limit && r.head < len(r.lines)-1 {
		r.bytes -= cost(r.lines[r.head].Content)
		r.lines[r.head] = guest.LogLine{}
		r.head++
	}

	// what is gone is let go of once it is half of what is held, which is
	// seldom enough that moving the rest costs nothing worth counting.
	if r.head > 0 && r.head >= len(r.lines)/2 {
		kept := copy(r.lines, r.lines[r.head:])
		clear(r.lines[kept:])
		r.lines = r.lines[:kept]
		r.head = 0
	}

	close(r.changed)
	r.changed = make(chan struct{})
}

// after is every line kept that came after the one numbered seq, and what is
// closed once there is another.
func (r *ring) after(seq uint64) ([]guest.LogLine, <-chan struct{}) {
	r.lock.Lock()
	defer r.lock.Unlock()

	kept := r.lines[r.head:]

	start := 0
	if len(kept) > 0 && seq >= kept[0].Seq {
		start = int(min(seq-kept[0].Seq+1, uint64(len(kept))))
	}

	return slices.Clone(kept[start:]), r.changed
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
