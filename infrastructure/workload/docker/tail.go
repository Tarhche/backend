package docker

import (
	"sync"
	"unicode/utf8"
)

// tailBuffer keeps the last of what is written to it, up to a limit, which is
// the part of a command's output that says how it ended. Several writers may
// share one, as a command's output and errors do.
type tailBuffer struct {
	lock  sync.Mutex
	limit int
	kept  []byte
}

func newTailBuffer(limit int) *tailBuffer {
	return &tailBuffer{limit: limit}
}

// Write never refuses what it is given: what is past the limit is dropped
// from the front, so the writer is never held up by a reader that is not
// there.
func (t *tailBuffer) Write(p []byte) (int, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	if len(p) >= t.limit {
		t.kept = append(t.kept[:0], p[len(p)-t.limit:]...)

		return len(p), nil
	}

	if over := len(t.kept) + len(p) - t.limit; over > 0 {
		t.kept = append(t.kept[:0], t.kept[over:]...)
	}

	t.kept = append(t.kept, p...)

	return len(p), nil
}

// String is what is kept, starting at a whole character: cutting the front
// off may have cut one in half.
func (t *tailBuffer) String() string {
	t.lock.Lock()
	defer t.lock.Unlock()

	kept := t.kept
	for len(kept) > 0 && !utf8.RuneStart(kept[0]) {
		kept = kept[1:]
	}

	return string(kept)
}
