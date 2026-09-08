package container

import (
	"encoding/json"
	"errors"
	"net/http"

	getContainer "github.com/khanzadimahdi/testproject/application/dashboard/runner/container/getContainer"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type showHandler struct {
	useCase *getContainer.UseCase
}

func NewShowHandler(useCase *getContainer.UseCase) *showHandler {
	return &showHandler{useCase: useCase}
}

// @Summary		Get container
// @Description	retrieve one container, with the addresses its ports are served on
// @Tags			dashboard runner
// @Accept			json
// @Produce		json
// @Param			uuid	path		string	true	"Container UUID"
// @Success		200		{object}	getContainer.Response
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/runner/containers/{uuid} [get]
func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), r.PathValue("uuid"))
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
