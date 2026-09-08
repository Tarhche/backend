package stack

import (
	"encoding/json"
	"errors"
	"net/http"

	getstack "github.com/khanzadimahdi/testproject/application/runner/manager/stack/getStack"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type showHandler struct {
	useCase *getstack.UseCase
}

func NewShowHandler(useCase *getstack.UseCase) *showHandler {
	return &showHandler{useCase: useCase}
}

// @Summary		Get stack
// @Description	retrieve a stack and the services in it
// @Tags			runner stacks
// @Accept			json
// @Produce		json
// @Param			uuid	path		string	true	"Stack UUID"
// @Success		200		{object}	getstack.Response
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Param			owner	query	string	false	"Only this owner's stack"
// @Router			/stacks/{uuid} [get]
func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	// an owner narrows it to that person's own, the way it narrows a listing:
	// a stack that is not theirs is not found.
	response, err := h.show(r, r.PathValue("uuid"))
	switch {
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

// show reads the stack, as anybody's or as one person's own.
func (h *showHandler) show(r *http.Request, UUID string) (*getstack.Response, error) {
	if ownerUUID := r.URL.Query().Get("owner"); len(ownerUUID) > 0 {
		return h.useCase.ExecuteOwn(r.Context(), ownerUUID, UUID)
	}

	return h.useCase.Execute(r.Context(), UUID)
}
