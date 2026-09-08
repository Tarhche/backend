package mock

import (
	"context"
	"sync"

	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain"
)

type MockReplyer struct {
	mock.Mock
}

var _ domain.Replyer = &MockReplyer{}

func (m *MockReplyer) Reply(ctx context.Context, reply *domain.Reply) error {
	args := m.Called(ctx, reply)

	return args.Error(0)
}

// RecordingReplyer keeps every reply it is given, so a test that is about what
// a stream sent can read it back rather than assert on each piece as it goes.
type RecordingReplyer struct {
	lock    sync.Mutex
	replies []domain.Reply

	// Fail, when set, is what every reply reports instead of being kept.
	Fail error
}

var _ domain.Replyer = &RecordingReplyer{}

func (r *RecordingReplyer) Reply(_ context.Context, reply *domain.Reply) error {
	if r.Fail != nil {
		return r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	r.replies = append(r.replies, *reply)

	return nil
}

// Replies is everything sent so far.
func (r *RecordingReplyer) Replies() []domain.Reply {
	r.lock.Lock()
	defer r.lock.Unlock()

	return append([]domain.Reply(nil), r.replies...)
}

// Chunks is the payload of every piece of a stream, in order.
func (r *RecordingReplyer) Chunks() [][]byte {
	var chunks [][]byte
	for _, reply := range r.Replies() {
		if reply.Kind == domain.ReplyChunk {
			chunks = append(chunks, reply.Payload)
		}
	}

	return chunks
}

// Ended reports whether the stream has been closed off.
func (r *RecordingReplyer) Ended() bool {
	for _, reply := range r.Replies() {
		if reply.Kind.EndsRequest() {
			return true
		}
	}

	return false
}
