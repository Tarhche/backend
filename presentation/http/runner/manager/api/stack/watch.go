package stack

import (
	"context"
	"log/slog"
	"net/http"

	getstack "github.com/khanzadimahdi/testproject/application/runner/manager/stack/getStack"
	watchstacks "github.com/khanzadimahdi/testproject/application/runner/manager/stack/watchStacks"
	"github.com/khanzadimahdi/testproject/presentation/http/runner/manager/api/internal/watch"
)

// NewWatchHandler follows the stacks the runner holds.
//
// @Summary		Watch stacks
// @Description	upgrades to a websocket carrying one message for each stack that changed, and one for each that is gone
// @Tags			runner stacks
// @Success		101	{string}	string	"switching protocols"
// @Router			/stacks/watch [get]
func NewWatchHandler(useCase *watchstacks.UseCase, logger *slog.Logger) http.Handler {
	return watch.Handler(watch.Watch[getstack.Response]{
		Field:    "stack",
		Identify: func(s *getstack.Response) string { return s.UUID },
		Poll: func(ctx context.Context) ([]getstack.Response, error) {
			response, err := useCase.Execute(ctx)
			if err != nil {
				return nil, err
			}

			return response.Items, nil
		},
		Logger: logger,
	})
}
