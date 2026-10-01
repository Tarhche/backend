package getTaskLogs

import (
	"context"

	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// defaultLimit is how many lines one read returns when the caller names no
// limit of its own.
const defaultLimit uint = 1000

// UseCase reads what a task has written, from its first line onward. The
// lines are kept against the task until it is deleted, so a stopped
// task still has its whole history.
type UseCase struct {
	workload workloadControlPlane.Client
}

func NewUseCase(workload workloadControlPlane.Client) *UseCase {
	return &UseCase{workload: workload}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {

	limit := request.Limit
	if limit == 0 {
		limit = defaultLimit
	}

	logs, err := uc.workload.TaskLogs(ctx, request.UUID, request.After, limit)
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
