// Package watchUserStacks keeps the dashboard's list of stacks as it is,
// without asking for it again.
//
// One request opens the watch and the replies to it are what changes: a message
// for each of their own stacks that changed, and one for each that is gone, until
// the client says it has seen enough. The changes themselves come from what the
// runner reports about its tasks, which every replica hears — this only
// says who is listening.
package watchUserStacks

import (
	"context"
	"encoding/json"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/runner/watch"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/gateway"
)

// WatchName is the subject a client opens one's own stacks watch on.
const WatchName = "runnerUserStacksWatch"

// UseCase registers the clients watching their own stacks.
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

	uc.watchers.WatchStacks(request.ID, auth.UUIDFromContext(ctx))

	// the watch lasts as long as the client does: a cancellation from whichever
	// replica the client walked away from is what ends it.
	uc.streams.Add(request.ID, func() { uc.watchers.Remove(request.ID) })

	return nil
}
