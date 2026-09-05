// Package shipLogs follows the containers running on this node and sends what
// they write to the manager, which keeps it. A container's log therefore
// outlives the container: it is held against the task until the task is
// deleted, rather than only until docker drops the container.
package shipLogs

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

const (
	// batchSize is how many lines are gathered before they are sent, so a
	// chatty container costs one message rather than one per line.
	batchSize = 64

	// batchWait is how long a partial batch waits for company before it is
	// sent anyway, so a quiet container's output is not held back.
	batchWait = 250 * time.Millisecond

	// retryWait is how long a follower waits before attaching again after its
	// stream ended unexpectedly.
	retryWait = time.Second
)

// UseCase keeps one follower per container running on this node.
type UseCase struct {
	containerManager container.Manager
	producer         domain.Producer
	nodeName         string
	logger           *slog.Logger

	lock      sync.Mutex
	followers map[string]context.CancelFunc
}

func NewUseCase(
	containerManager container.Manager,
	producer domain.Producer,
	nodeName string,
	logger *slog.Logger,
) *UseCase {
	return &UseCase{
		containerManager: containerManager,
		producer:         producer,
		nodeName:         nodeName,
		logger:           logger,
		followers:        make(map[string]context.CancelFunc),
	}
}

// Execute brings the followers in line with what is running: a container that
// has appeared is followed, and one that has gone is let go. It is called
// repeatedly, so it does only the difference each time.
func (uc *UseCase) Execute(ctx context.Context) error {
	containers, err := uc.containerManager.GetByLabel(ctx, container.NodeNameLabelKey, uc.nodeName)
	if err != nil {
		return err
	}

	present := make(map[string]struct{}, len(containers))

	for _, c := range containers {
		// a job's whole log rides its heartbeat, which is what the code runner
		// waits for. Only a long-running service needs its log streamed.
		if task.Kind(c.Labels[container.TaskKindLabelKey]) != task.KindService {
			continue
		}

		taskUUID := c.Labels[container.TaskUUIDLabelKey]
		if len(taskUUID) == 0 {
			continue
		}

		present[c.ID] = struct{}{}
		uc.follow(ctx, c.ID, taskUUID)
	}

	uc.releaseAbsent(present)

	return nil
}

// Close lets go of every container this node was following.
func (uc *UseCase) Close() {
	uc.releaseAbsent(nil)
}

// following reports how many containers this node is currently following.
func (uc *UseCase) following() int {
	uc.lock.Lock()
	defer uc.lock.Unlock()

	return len(uc.followers)
}

// follow starts one follower for a container, if there is not one already.
func (uc *UseCase) follow(ctx context.Context, containerID string, taskUUID string) {
	uc.lock.Lock()
	defer uc.lock.Unlock()

	if _, already := uc.followers[containerID]; already {
		return
	}

	// detached from the call that discovered the container: a follower lives
	// for as long as the container does, not for the length of one sweep.
	followerCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	uc.followers[containerID] = cancel

	go func() {
		defer func() {
			uc.lock.Lock()
			delete(uc.followers, containerID)
			uc.lock.Unlock()
		}()

		uc.stream(followerCtx, containerID, taskUUID)
	}()
}

// releaseAbsent stops following whatever is no longer present.
func (uc *UseCase) releaseAbsent(present map[string]struct{}) {
	uc.lock.Lock()
	cancels := make([]context.CancelFunc, 0, len(uc.followers))
	for containerID, cancel := range uc.followers {
		if _, still := present[containerID]; !still {
			cancels = append(cancels, cancel)
		}
	}
	uc.lock.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
}

// stream follows one container until it is let go, attaching again if its
// stream ends while the container is still there.
func (uc *UseCase) stream(ctx context.Context, containerID string, taskUUID string) {
	// resumed from the last line shipped, so a stream that has to be picked up
	// again does not start over. The lines around that moment arrive twice and
	// the manager stores each of them once.
	var since time.Time

	for ctx.Err() == nil {
		batch := newBatch(uc, containerID, taskUUID)

		// a batch is sent when it is full or when it has waited long enough,
		// and the waiting has to be its own clock: a container that writes a
		// burst and then goes quiet would otherwise hold its last lines until
		// it happened to write again.
		sending := uc.sendPeriodically(ctx, batch)

		err := uc.containerManager.StreamLogs(ctx, containerID, since, func(line container.LogLine) error {
			since = line.At

			return batch.add(ctx, line)
		})

		sending()
		batch.flush(ctx)

		if ctx.Err() != nil {
			return
		}

		if err != nil {
			uc.logger.WarnContext(ctx, "a container's log stream ended", "error", err, "containerID", containerID)
		}

		select {
		case <-time.After(retryWait):
		case <-ctx.Done():
			return
		}
	}
}

// sendPeriodically keeps sending whatever a batch has gathered, until the
// returned function is called.
func (uc *UseCase) sendPeriodically(ctx context.Context, b *batch) func() {
	ticking, stop := context.WithCancel(ctx)

	go func() {
		ticker := time.NewTicker(batchWait)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				b.flush(ctx)
			case <-ticking.Done():
				return
			}
		}
	}()

	return stop
}

// batch gathers lines so a chatty container costs one message rather than one
// per line. It is filled by the goroutine reading the container's output and
// emptied by that one or by the clock, so it holds a lock of its own.
type batch struct {
	useCase     *UseCase
	containerID string
	taskUUID    string

	lock  sync.Mutex
	lines []events.LogLine
}

func newBatch(useCase *UseCase, containerID string, taskUUID string) *batch {
	return &batch{
		useCase:     useCase,
		containerID: containerID,
		taskUUID:    taskUUID,
		lines:       make([]events.LogLine, 0, batchSize),
	}
}

func (b *batch) add(ctx context.Context, line container.LogLine) error {
	b.lock.Lock()
	b.lines = append(b.lines, events.LogLine{
		Stream:  uint8(line.Stream),
		Content: line.Content,
		At:      line.At,
	})
	full := len(b.lines) >= batchSize
	b.lock.Unlock()

	if full {
		b.flush(ctx)
	}

	return ctx.Err()
}

// flush sends what has been gathered, if anything has.
func (b *batch) flush(ctx context.Context) {
	b.lock.Lock()
	lines := b.lines
	b.lines = make([]events.LogLine, 0, batchSize)
	b.lock.Unlock()

	if len(lines) == 0 {
		return
	}

	event := events.TaskLogged{
		UUID:          b.taskUUID,
		ContainerUUID: b.containerID,
		NodeName:      b.useCase.nodeName,
		Lines:         lines,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		b.useCase.logger.ErrorContext(ctx, "error on marshalling a log batch", "error", err)

		return
	}

	// detached from the follower's own context, so the last batch of a
	// container being let go still reaches the manager.
	if err := b.useCase.producer.Produce(context.WithoutCancel(ctx), events.TaskLoggedName, payload); err != nil {
		b.useCase.logger.ErrorContext(ctx, "error on shipping a log batch", "error", err, "containerID", b.containerID)
	}
}
