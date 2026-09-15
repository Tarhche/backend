package stack

import (
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	deleteuserstack "github.com/khanzadimahdi/testproject/application/dashboard/runner/stack/deleteUserStack"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type deleteUserHandler struct {
	useCase *deleteuserstack.UseCase
}

func NewDeleteUserHandler(useCase *deleteuserstack.UseCase) *deleteUserHandler {
	return &deleteUserHandler{useCase: useCase}
}

// @Summary		Delete own stack
// @Description	stop and remove one of your own stacks
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Stack UUID"
// @Success		204		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/my/runner/stacks/{uuid} [delete]
func (h *deleteUserHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	err := h.useCase.Execute(r.Context(), &deleteuserstack.Request{
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
		rw.WriteHeader(http.StatusNoContent)
	}
}
