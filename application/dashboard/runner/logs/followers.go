// Package logs carries a task's output to the clients watching it.
//
// A task's lines are already published as it writes them — an orchestrator ships
// them, the control plane stores them — so following one is hearing that, not asking
// anybody over and over. Every replica hears every line and answers for the
// clients it is holding.
//
// What a client is told first is what the task had already written, read
// once from the runner. Lines that arrive while that is being read are kept
// until it has been sent, so a follower sees the whole of it, in order, with
// nothing twice.
package logs

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// maxPending bounds what is kept for a follower that is still catching up.
// Reading a backlog takes one call to the runner, so this only has to cover
// what a task can write meanwhile; past it, the newest lines are the ones
// worth keeping.
const maxPending = 2048

// Followers is who is being sent a task's output, and how far each of them
// has been told.
type Followers struct {
	lock    sync.Mutex
	open    map[string]*Follower
	replyer domain.Replyer
	logger  *slog.Logger
}

func NewFollowers(replyer domain.Replyer, logger *slog.Logger) *Followers {
	return &Followers{
		open:    make(map[string]*Follower),
		replyer: replyer,
		logger:  logger,
	}
}

// Follow registers a client following a task from a moment onward. It is
// catching up until CatchUp says otherwise: whatever the task writes
// meanwhile is kept rather than sent, because what it wrote before that has not
// been sent yet.
func (f *Followers) Follow(requestID string, taskUUID string, after time.Time) *Follower {
	follower := &Follower{
		followers:  f,
		requestID:  requestID,
		taskUUID:   taskUUID,
		cursor:     after,
		catchingUp: true,
	}

	f.lock.Lock()
	defer f.lock.Unlock()

	f.open[requestID] = follower

	return follower
}

// Remove forgets a follower whose client is no longer there.
func (f *Followers) Remove(requestID string) {
	f.lock.Lock()
	defer f.lock.Unlock()

	delete(f.open, requestID)
}

// Len reports how many follows this replica is holding.
func (f *Followers) Len() int {
	f.lock.Lock()
	defer f.lock.Unlock()

	return len(f.open)
}

// of is everybody following one task.
func (f *Followers) of(taskUUID string) []*Follower {
	f.lock.Lock()
	defer f.lock.Unlock()

	followers := make([]*Follower, 0, len(f.open))
	for _, follower := range f.open {
		if follower.taskUUID == taskUUID {
			followers = append(followers, follower)
		}
	}

	return followers
}

// Follower is one client, following one task.
type Follower struct {
	followers *Followers
	requestID string
	taskUUID  string

	lock sync.Mutex

	// cursor is the moment of the last line this follower was sent, so a line
	// it has already seen is not sent again.
	cursor time.Time

	catchingUp bool
	pending    []task.LogLine
}

// CatchUp sends what the task had already written and then whatever it
// wrote while that was being read. From here on the follower is told about a
// line as it arrives.
func (f *Follower) CatchUp(ctx context.Context, backlog []task.Log) {
	f.lock.Lock()
	defer f.lock.Unlock()

	for i := range backlog {
		f.send(ctx, backlog[i].LogLine)
	}

	pending := f.pending

	f.pending = nil
	f.catchingUp = false

	for i := range pending {
		f.send(ctx, pending[i])
	}
}

// write is a batch of lines the task has just written.
func (f *Follower) write(ctx context.Context, lines []task.LogLine) {
	f.lock.Lock()
	defer f.lock.Unlock()

	if f.catchingUp {
		f.keep(lines)

		return
	}

	for i := range lines {
		f.send(ctx, lines[i])
	}
}

// keep holds a line until the backlog before it has been sent.
func (f *Follower) keep(lines []task.LogLine) {
	f.pending = append(f.pending, lines...)

	if len(f.pending) > maxPending {
		f.pending = f.pending[len(f.pending)-maxPending:]
	}
}

// send carries one line, unless it is one this follower has already seen.
func (f *Follower) send(ctx context.Context, line task.LogLine) {
	if !line.At.After(f.cursor) {
		return
	}

	payload, err := json.Marshal(Response{
		Stream:  line.Stream.String(),
		Content: line.Content,
		At:      line.At,
	})
	if err != nil {
		f.followers.logger.ErrorContext(ctx, "error on marshalling a log line", "error", err)

		return
	}

	if err := f.followers.replyer.Reply(ctx, &domain.Reply{
		RequestID: f.requestID,
		Kind:      domain.ReplyChunk,
		Payload:   payload,
	}); err != nil {
		f.followers.logger.ErrorContext(ctx, "error on writing a log line", "error", err, "requestID", f.requestID)

		return
	}

	f.cursor = line.At
}
