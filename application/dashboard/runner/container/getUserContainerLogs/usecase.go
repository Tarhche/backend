package getusercontainerlogs

import (
	"context"

	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
)

// defaultLimit is how many lines one read returns when the caller names no
// limit of its own.
const defaultLimit uint = 500

// UseCase reads what one of somebody's own containers has written, from its
// first line onward. The lines are kept against the container until it is
// deleted, so a stopped container still has its whole history.
type UseCase struct {
	runner runnerManager.Client
}

func NewUseCase(runner runnerManager.Client) *UseCase {
	return &UseCase{runner: runner}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	// read as theirs first: a container that is somebody else's has no log
	// here, which is what somebody who may only read their own is told.
	if _, err := uc.runner.ContainerOf(ctx, request.OwnerUUID, request.UUID); err != nil {
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
