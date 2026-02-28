package task

import (
	"encoding/json"
	"errors"
	"net/http"

	stoptask "github.com/khanzadimahdi/testproject/application/runner/manager/task/stopTask"
	"github.com/khanzadimahdi/testproject/domain"
)

type stopHandler struct {
	useCase *stoptask.UseCase
}

func NewStopHandler(useCase *stoptask.UseCase) *stopHandler {
	return &stopHandler{
		useCase: useCase,
	}
}

// @Summary		Stop task
// @Description	signal a running task to stop
// @Tags			runner tasks
// @Accept			json
// @Produce		json
// @Param			uuid	path		string	true	"Task UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/tasks/{uuid}/stop [post]
func (h *stopHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	UUID := r.PathValue("uuid")
	request := &stoptask.Request{
		UUID: UUID,
	}

	response, err := h.useCase.Execute(request)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	case response != nil && len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.WriteHeader(http.StatusAccepted)
	}
}
