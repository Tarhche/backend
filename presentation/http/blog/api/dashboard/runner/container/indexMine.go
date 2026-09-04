package container

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/khanzadimahdi/testproject/application/auth"
	getContainers "github.com/khanzadimahdi/testproject/application/dashboard/runner/container/getContainers"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

// indexMineHandler lists the containers the person asking owns. It is the same
// listing as everybody's, narrowed to one owner, so the two read and page
// alike.
type indexMineHandler struct {
	useCase *getContainers.UseCase
}

func NewIndexMineHandler(useCase *getContainers.UseCase) *indexMineHandler {
	return &indexMineHandler{useCase: useCase}
}

// @Summary		List my containers
// @Description	paginated list of the containers the current user owns
// @Tags			dashboard runner
// @Accept			json
// @Produce		json
// @Param			page	query		int	false	"Page"	default(1)
// @Success		200		{object}	getContainers.Response
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/my/runner/containers [get]
func (h *indexMineHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var page uint = 1
	if parsed, err := strconv.ParseUint(r.URL.Query().Get("page"), 10, 32); err == nil {
		page = uint(parsed)
	}

	response, err := h.useCase.Execute(r.Context(), &getContainers.Request{
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
