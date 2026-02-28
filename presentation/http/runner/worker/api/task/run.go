package task

import (
	"encoding/json"
	"errors"
	"net/http"

	runtask "github.com/khanzadimahdi/testproject/application/runner/worker/task/runTask"
	"github.com/khanzadimahdi/testproject/domain"
)

type runHandler struct {
	useCase *runtask.UseCase
}

func NewRunHandler(useCase *runtask.UseCase) *runHandler {
	return &runHandler{
		useCase: useCase,
	}
}

// @Summary		Run worker task
// @Description	accept a task result or status update from worker
// @Tags			runner tasks
// @Accept			json
// @Produce		json
// @Param			body	body		runtask.Request	true	"Task operation"
// @Success		202		{object}	map[string]interface{}
// @Failure		400		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/tasks/run [post]
func (h *runHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request runtask.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	response, err := h.useCase.Execute(&request)
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
