package stack

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/khanzadimahdi/testproject/application/auth"
	getStacks "github.com/khanzadimahdi/testproject/application/dashboard/runner/stack/getStacks"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

// indexMineHandler lists the stacks the person asking owns. It is the same
// listing as everybody's, narrowed to one owner, so the two read and page
// alike.
type indexMineHandler struct {
	useCase *getStacks.UseCase
}

func NewIndexMineHandler(useCase *getStacks.UseCase) *indexMineHandler {
	return &indexMineHandler{useCase: useCase}
}

// @Summary		List my stacks
// @Description	paginated list of the stacks the current user owns
// @Tags			dashboard runner
// @Accept			json
// @Produce		json
// @Param			page	query		int	false	"Page"	default(1)
// @Success		200		{object}	getStacks.Response
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/my/runner/stacks [get]
func (h *indexMineHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var page uint = 1
	if parsed, err := strconv.ParseUint(r.URL.Query().Get("page"), 10, 32); err == nil {
		page = uint(parsed)
	}

	response, err := h.useCase.Execute(r.Context(), &getStacks.Request{
		Page:      page,
		OwnerUUID: auth.UUIDFromContext(r.Context()),
	})
	switch {
	case errors.Is(err, domain.ErrForbidden):
		rw.WriteHeader(http.StatusForbidden)
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
