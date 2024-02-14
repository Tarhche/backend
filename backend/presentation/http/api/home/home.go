package article

import (
	"encoding/json"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/home"
)

type homeHandler struct {
	homeUseCase *home.UseCase
}

func NewHomeHandler(homeUseCase *home.UseCase) *homeHandler {
	return &homeHandler{
		homeUseCase: homeUseCase,
	}
}

func (h *homeHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.homeUseCase.Execute()
	switch true {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
