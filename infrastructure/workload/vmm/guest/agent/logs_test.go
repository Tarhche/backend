package agent

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
)

func TestRing(t *testing.T) {
	t.Run("lines are numbered in the order they came, from one", func(t *testing.T) {
		r := newRing(1 << 10)
		r.add(guest.StreamStdout, "first", time.Now())
		r.add(guest.StreamStderr, "second", time.Now())

		lines, _ := r.after(0)

		require.Len(t, lines, 2)
		assert.Equal(t, uint64(1), lines[0].Seq)
		assert.Equal(t, "second", lines[1].Content)
		assert.Equal(t, guest.StreamStderr, lines[1].Stream)

		lines, _ = r.after(1)
		require.Len(t, lines, 1)
		assert.Equal(t, uint64(2), lines[0].Seq)

		lines, _ = r.after(2)
		assert.Empty(t, lines)

		lines, _ = r.after(100)
		assert.Empty(t, lines, "nothing has come after a line that was never written")
	})

	t.Run("the oldest lines go once there is no room, and their numbers with them", func(t *testing.T) {
		r := newRing(2 * cost("12345"))
		r.add(guest.StreamStdout, "12345", time.Now())
		r.add(guest.StreamStdout, "67890", time.Now())
		r.add(guest.StreamStdout, "abcde", time.Now())

		lines, _ := r.after(0)

		require.Len(t, lines, 2)
		assert.Equal(t, uint64(2), lines[0].Seq, "a reader sees the gap in the numbers")

		lines, _ = r.after(1)
		require.Len(t, lines, 2, "a reader that saw a line that is gone gets everything kept after it")
		assert.Equal(t, uint64(2), lines[0].Seq)
	})

	t.Run("a line larger than the whole ring is still kept", func(t *testing.T) {
		r := newRing(4)
		r.add(guest.StreamStdout, "much too long", time.Now())

		lines, _ := r.after(0)
		require.Len(t, lines, 1)
	})

	t.Run("lines are found by their number however much has gone before them", func(t *testing.T) {
		r := newRing(10 * cost("line 000"))

		for i := range 1000 {
			r.add(guest.StreamStdout, "line "+strconv.Itoa(1000 + i)[1:], time.Now())
		}

		lines, _ := r.after(0)
		require.Len(t, lines, 10)
		assert.Equal(t, uint64(991), lines[0].Seq)
		assert.Equal(t, "line 999", lines[9].Content)

		for seq := uint64(990); seq <= 1000; seq++ {
			lines, _ := r.after(seq)

			require.Len(t, lines, int(1000-max(seq, 990)))
			if len(lines) > 0 {
				assert.Equal(t, max(seq, 990)+1, lines[0].Seq)
			}
		}
	})

	t.Run("short lines are held to the ring's size as long ones are", func(t *testing.T) {
		r := newRing(64 << 10)

		for range 100_000 {
			r.add(guest.StreamStdout, "x", time.Now())
		}

		lines, _ := r.after(0)

		assert.LessOrEqual(t, len(lines)*cost("x"), 64<<10)
		assert.Equal(t, uint64(100_000), lines[len(lines)-1].Seq)
		assert.LessOrEqual(t, len(r.lines), 2*len(lines)+1, "what is gone does not pile up behind what is kept")
	})

	t.Run("a full ring takes a line in without moving all the others", func(t *testing.T) {
		r := newRing(defaultLogBytes)
		line := strings.Repeat("x", 20)

		start := time.Now()
		for range 1_000_000 {
			r.add(guest.StreamStdout, line, start)
		}

		// a ring that moved everything it holds for every line it takes in
		// spends minutes on this; one that does not, well under a second.
		assert.Less(t, time.Since(start), 20*time.Second)

		lines, _ := r.after(0)
		assert.Equal(t, uint64(1_000_000), lines[len(lines)-1].Seq)
	})

	t.Run("whoever waits is told when a line comes", func(t *testing.T) {
		r := newRing(1 << 10)

		_, changed := r.after(0)

		select {
		case <-changed:
			t.Fatal("nothing came yet")
		default:
		}

		r.add(guest.StreamStdout, "now", time.Now())

		select {
		case <-changed:
		case <-time.After(time.Second):
			t.Fatal("whoever waited was not told")
		}
	})

	t.Run("a stream is read line by line, and a line too long is kept in pieces", func(t *testing.T) {
		r := newRing(1 << 20)

		long := strings.Repeat("x", logLineLimit+10)
		r.pump(guest.StreamStdout, strings.NewReader("one\r\ntwo\n"+long+"\nlast without a newline"))

		lines, _ := r.after(0)

		contents := make([]string, len(lines))
		for i, line := range lines {
			contents[i] = line.Content
		}

		assert.Equal(t, []string{"one", "two", long[:logLineLimit], long[logLineLimit:], "last without a newline"}, contents)
	})
}
