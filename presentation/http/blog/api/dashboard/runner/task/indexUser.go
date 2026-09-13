package task

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/khanzadimahdi/testproject/application/auth"
	getusertasks "github.com/khanzadimahdi/testproject/application/dashboard/runner/task/getUserTasks"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

// indexUserHandler lists the tasks the person asking owns. It is the same
// listing as everybody's, narrowed to one owner, so the two read and page
// alike.
type indexUserHandler struct {
	useCase *getusertasks.UseCase
}

func NewIndexUserHandler(useCase *getusertasks.UseCase) *indexUserHandler {
	return &indexUserHandler{useCase: useCase}
}

// @Summary		List my tasks
// @Description	paginated list of the tasks the current user owns
// @Tags			dashboard runner
// @Accept			json
// @Produce		json
// @Param			page	query		int	false	"Page"	default(1)
// @Success		200		{object}	getusertasks.Response
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/my/runner/tasks [get]
func (h *indexUserHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var page uint = 1
	if parsed, err := strconv.ParseUint(r.URL.Query().Get("page"), 10, 32); err == nil {
		page = uint(parsed)
	}

	response, err := h.useCase.Execute(r.Context(), &getusertasks.Request{
		Page:      page,
		OwnerUUID: auth.UUIDFromContext(r.Context()),
	})
	switch {
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
