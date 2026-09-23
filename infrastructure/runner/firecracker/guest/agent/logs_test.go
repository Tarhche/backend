package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/guest"
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
	})

	t.Run("the oldest lines go once there is no room, and their numbers with them", func(t *testing.T) {
		r := newRing(10)
		r.add(guest.StreamStdout, "12345", time.Now())
		r.add(guest.StreamStdout, "67890", time.Now())
		r.add(guest.StreamStdout, "abcde", time.Now())

		lines, _ := r.after(0)

		require.Len(t, lines, 2)
		assert.Equal(t, uint64(2), lines[0].Seq, "a reader sees the gap in the numbers")
	})

	t.Run("a line larger than the whole ring is still kept", func(t *testing.T) {
		r := newRing(4)
		r.add(guest.StreamStdout, "much too long", time.Now())

		lines, _ := r.after(0)
		require.Len(t, lines, 1)
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
