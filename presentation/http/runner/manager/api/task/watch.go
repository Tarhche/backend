package task

import (
	"context"
	"log/slog"
	"net/http"

	gettask "github.com/khanzadimahdi/testproject/application/runner/manager/task/getTask"
	watchtasks "github.com/khanzadimahdi/testproject/application/runner/manager/task/watchTasks"
	"github.com/khanzadimahdi/testproject/presentation/http/runner/manager/api/internal/watch"
)

// NewWatchHandler follows the containers the runner holds.
//
// @Summary		Watch containers
// @Description	upgrades to a websocket carrying one message for each container whose state changed, and one for each that is gone
// @Tags			runner tasks
// @Success		101	{string}	string	"switching protocols"
// @Router			/tasks/watch [get]
func NewWatchHandler(useCase *watchtasks.UseCase, logger *slog.Logger) http.Handler {
	return watch.Handler(watch.Watch[gettask.Response]{
		Field:    "task",
		Identify: func(t *gettask.Response) string { return t.UUID },
		Poll: func(ctx context.Context) ([]gettask.Response, error) {
			response, err := useCase.Execute(ctx)
			if err != nil {
				return nil, err
			}

			return response.Items, nil
		},
		Logger: logger,
	})
}
