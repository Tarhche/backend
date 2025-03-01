package task

import (
	"encoding/json"
	"errors"
	"net/http"

	stoptask "github.com/khanzadimahdi/testproject/application/runner/worker/task/stopTask"
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

func (h *stopHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := stoptask.Request{
		UUID: r.PathValue("uuid"),
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
