package home

import (
	"encoding/json"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/home"
)

type homeHandler struct {
	useCase *home.UseCase
}

func NewHomeHandler(useCase *home.UseCase) *homeHandler {
	return &homeHandler{
		useCase: useCase,
	}
}

func (h *homeHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
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
