package stack

import (
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	restartuserstack "github.com/khanzadimahdi/testproject/application/dashboard/runner/stack/restartUserStack"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type restartUserHandler struct {
	useCase *restartuserstack.UseCase
}

func NewRestartUserHandler(useCase *restartuserstack.UseCase) *restartUserHandler {
	return &restartUserHandler{useCase: useCase}
}

// @Summary		Restart own stack
// @Description	restart every service of one of your own stacks
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Stack UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/my/runner/stacks/{uuid}/restart [post]
func (h *restartUserHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	err := h.useCase.Execute(r.Context(), &restartuserstack.Request{
		UUID:      r.PathValue("uuid"),
		OwnerUUID: auth.UUIDFromContext(r.Context()),
	})

	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.WriteHeader(http.StatusAccepted)
	}
}
