package getContainerLogs

import (
	"context"
	"time"

	runnerAccess "github.com/khanzadimahdi/testproject/application/dashboard/runner/access"
	"github.com/khanzadimahdi/testproject/domain/permission"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
)

// defaultLimit is how many lines one read returns when the caller names no
// limit of its own.
const defaultLimit uint = 500

type Request struct {
	UUID string `json:"-"`

	// ActorUUID is who is asking, which decides whether this container's log
	// is theirs to read.
	ActorUUID string `json:"-"`

	After time.Time `json:"after"`
	Limit uint      `json:"limit"`
}

type Response struct {
	Items []LogResponse `json:"items"`
}

type LogResponse struct {
	Stream  string    `json:"stream"`
	Content string    `json:"content"`
	At      time.Time `json:"at"`
}

// UseCase reads what a container has written, from its first line onward. The
// lines are kept against the container until it is deleted, so a stopped
// container still has its whole history.
type UseCase struct {
	runner runnerManager.Client
	guard  *runnerAccess.Guard
}

func NewUseCase(runner runnerManager.Client, guard *runnerAccess.Guard) *UseCase {
	return &UseCase{runner: runner, guard: guard}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if err := uc.guard.OverContainer(ctx, request.ActorUUID, permission.RunnerContainersLogs, permission.SelfRunnerContainersLogs, request.UUID); err != nil {
		return nil, err
	}

	limit := request.Limit
	if limit == 0 {
		limit = defaultLimit
	}

	logs, err := uc.runner.ContainerLogs(ctx, request.UUID, request.After, limit)
	if err != nil {
		return nil, err
	}

	items := make([]LogResponse, len(logs))
	for i, l := range logs {
		items[i] = LogResponse{
			Stream:  l.Stream.String(),
			Content: l.Content,
			At:      l.At,
		}
	}

	return &Response{Items: items}, nil
}
