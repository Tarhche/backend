package task

import (
	"encoding/json"
	"errors"
	"net/http"

	deletetask "github.com/khanzadimahdi/testproject/application/runner/manager/task/deleteTask"
	"github.com/khanzadimahdi/testproject/domain"
)

type deleteHandler struct {
	useCase *deletetask.UseCase
}

func NewDeleteHandler(useCase *deletetask.UseCase) *deleteHandler {
	return &deleteHandler{
		useCase: useCase,
	}
}

func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	UUID := r.PathValue("uuid")
	request := &deletetask.Request{
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
		rw.WriteHeader(http.StatusNoContent)
	}
}
