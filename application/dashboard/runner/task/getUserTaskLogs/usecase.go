package getusertasklogs

import (
	"context"

	runnerControlPlane "github.com/khanzadimahdi/testproject/domain/runner/controlplane"
)

// defaultLimit is how many lines one read returns when the caller names no
// limit of its own.
const defaultLimit uint = 1000

// UseCase reads what one of somebody's own tasks has written, from its
// first line onward. The lines are kept against the task until it is
// deleted, so a stopped task still has its whole history.
type UseCase struct {
	runner runnerControlPlane.Client
}

func NewUseCase(runner runnerControlPlane.Client) *UseCase {
	return &UseCase{runner: runner}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	// read as theirs first: a task that is somebody else's has no log
	// here, which is what somebody who may only read their own is told.
	if _, err := uc.runner.TaskOf(ctx, request.OwnerUUID, request.UUID); err != nil {
		return nil, err
	}

	limit := request.Limit
	if limit == 0 {
		limit = defaultLimit
	}

	logs, err := uc.runner.TaskLogs(ctx, request.UUID, request.After, limit)
	if err != nil {
		return nil, err
	}

	items := make([]log, len(logs))
	for i, l := range logs {
		items[i] = log{
			Stream:  l.Stream.String(),
			Content: l.Content,
			At:      l.At,
		}
	}

	return &Response{Items: items}, nil
}
