package watchUserTasks

import (
	"context"
	"encoding/json"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/runner/watch"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/gateway"
)

// WatchName is the subject a client opens one's own tasks watch on.
const WatchName = "runnerUserTasksWatch"

// UseCase registers the clients watching their own tasks.
type UseCase struct {
	watchers *watch.Watchers
	streams  *gateway.Streams
}

var _ domain.MessageHandler = &UseCase{}

func NewUseCase(watchers *watch.Watchers, streams *gateway.Streams) *UseCase {
	return &UseCase{watchers: watchers, streams: streams}
}

func (uc *UseCase) Handle(ctx context.Context, data []byte) error {
	var request Request
	if err := json.Unmarshal(data, &request); err != nil {
		return nil
	}

	uc.watchers.WatchTasks(request.ID, auth.UUIDFromContext(ctx))

	// the watch lasts as long as the client does: a cancellation from whichever
	// replica the client walked away from is what ends it.
	uc.streams.Add(request.ID, func() { uc.watchers.Remove(request.ID) })

	return nil
}
