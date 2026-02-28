package task

import (
	"encoding/json"
	"errors"
	"net/http"

	gettask "github.com/khanzadimahdi/testproject/application/runner/manager/task/getTask"
	"github.com/khanzadimahdi/testproject/domain"
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
// @Router			/tasks/{uuid} [get]
func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	UUID := r.PathValue("uuid")

	response, err := h.useCase.Execute(UUID)

	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
