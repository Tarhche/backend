package tag

import (
	"encoding/json"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/home"
)

type tagHandler struct {
	tagUseCase *home.UseCase
}

func NewTagHandler(tagUseCase *home.UseCase) *tagHandler {
	return &tagHandler{
		tagUseCase: tagUseCase,
	}
}

func (h *tagHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.tagUseCase.Execute()
	switch true {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
