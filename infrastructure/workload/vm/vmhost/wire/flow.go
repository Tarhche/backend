package wire

import (
	"bytes"
	"fmt"
	"sync"
)

// Credit is the sending side of one stream's flow control: how much more of
// the stream may be sent before the other side says it has read some of it.
// It starts at a Window.
type Credit struct {
	lock sync.Mutex
	more sync.Cond
	room int
	err  error
}

func NewCredit() *Credit {
	c := &Credit{room: Window}
	c.more.L = &c.lock

	return c
}

// Take waits for room to send in and takes up to most of it. Whatever of it
// is not sent after all is given back with Grant.
func (c *Credit) Take(most int) (int, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	for c.room == 0 && c.err == nil {
		c.more.Wait()
	}

	if c.err != nil {
		return 0, c.err
	}

	n := min(most, c.room)
	c.room -= n

	return n, nil
}

// Grant is room for n more bytes, which a window frame gives, or which a
// sender gives back when it sent less than it took. More room than a window
// is a receiver that granted what it was never sent.
func (c *Credit) Grant(n int) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.err != nil {
		return nil
	}

	c.room += n
	if c.room > Window {
		return fmt.Errorf("%w: room for %d bytes of a stream, more than a window", ErrProtocol, c.room)
	}

	c.more.Broadcast()

	return nil
}

// Fail ends the stream for its sender: whoever waits for room, and whoever
// asks for it later, is told err.
func (c *Credit) Fail(err error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.err == nil {
		c.err = err
	}

	c.more.Broadcast()
}

// Inbound is the receiving side of one stream: what has arrived of it and has
// not been read yet. Reading it gives the sender its room back, half a window
// at a time, so a sender never has more than a window outstanding and one
// that is waiting for room is always given it once its stream is read.
type Inbound struct {
	lock sync.Mutex
	more sync.Cond
	data bytes.Buffer

	// err is why the stream ends, once what is buffered has been read.
	err error

	// read is how much has been read since the sender was last given room.
	read int

	// grant gives the sender room for that many more bytes.
	grant func(n int)
}

// NewInbound is a stream whose reading gives its sender room through grant.
func NewInbound(grant func(n int)) *Inbound {
	b := &Inbound{grant: grant}
	b.more.L = &b.lock

	return b
}

// Push adds what arrived. More than a window of the stream unread is a sender
// that did not wait for room. What arrives after the stream has ended is
// dropped.
func (b *Inbound) Push(p []byte) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.err != nil {
		return nil
	}

	if b.data.Len()+len(p) > Window {
		return fmt.Errorf("%w: more than a window of a stream sent", ErrProtocol)
	}

	b.data.Write(p)
	b.more.Broadcast()

	return nil
}

// End ends the stream once what has arrived has been read: with io.EOF at its
// end, or with why it ended early. Only the first end counts.
func (b *Inbound) End(err error) {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.err == nil {
		b.err = err
	}

	b.more.Broadcast()
}

// Fail ends the stream at once, with what has not been read yet dropped:
// from now on reading it says err.
func (b *Inbound) Fail(err error) {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.data.Reset()
	b.err = err
	b.more.Broadcast()
}

func (b *Inbound) Read(p []byte) (int, error) {
	b.lock.Lock()

	if len(p) == 0 && b.err == nil {
		b.lock.Unlock()

		return 0, nil
	}

	for b.data.Len() == 0 && b.err == nil {
		b.more.Wait()
	}

	if b.data.Len() == 0 {
		err := b.err
		b.lock.Unlock()

		return 0, err
	}

	n, _ := b.data.Read(p)

	b.read += n

	var room int
	if b.read >= Window/2 {
		room, b.read = b.read, 0
	}

	b.lock.Unlock()

	// given outside the lock: giving it is a write to the connection, which
	// is not to hold up whoever pushes what arrives meanwhile.
	if room > 0 && b.grant != nil {
		b.grant(room)
	}

	return n, nil
}
