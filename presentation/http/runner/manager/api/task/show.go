package task

import (
	"encoding/json"
	"errors"
	"net/http"

	gettask "github.com/khanzadimahdi/testproject/application/runner/manager/task/getTask"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type showHandler struct {
	useCase *gettask.UseCase
}

func NewShowHandler(useCase *gettask.UseCase) *showHandler {
	return &showHandler{
		useCase: useCase,
	}
}

// @Summary		Get task
// @Description	retrieve details of a task
// @Tags			runner tasks
// @Accept			json
// @Produce		json
// @Param			uuid	path		string	true	"Task UUID"
// @Success		200		{object}	gettask.Response
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Param			owner	query	string	false	"Only this owner's container"
// @Router			/tasks/{uuid} [get]
func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	UUID := r.PathValue("uuid")

	// an owner narrows it to that person's own, the way it narrows a listing:
	// a container that is not theirs is not found.
	response, err := h.show(r, UUID)

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

// show reads the container, as anybody's or as one person's own.
func (h *showHandler) show(r *http.Request, UUID string) (*gettask.Response, error) {
	if ownerUUID := r.URL.Query().Get("owner"); len(ownerUUID) > 0 {
		return h.useCase.ExecuteOwn(r.Context(), ownerUUID, UUID)
	}

	return h.useCase.Execute(r.Context(), UUID)
}
