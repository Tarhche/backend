package task

import (
	"encoding/json"
	"net/http"

	gettasks "github.com/khanzadimahdi/testproject/application/runner/worker/task/getTasks"
)

type indexHandler struct {
	useCase *gettasks.UseCase
}

func NewIndexHandler(useCase *gettasks.UseCase) *indexHandler {
	return &indexHandler{
		useCase: useCase,
	}
}

func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute()
	switch {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
