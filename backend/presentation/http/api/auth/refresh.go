package auth

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth/refresh"
	"github.com/khanzadimahdi/testproject/domain"
)

type refreshHandler struct {
	refreshUseCase *refresh.UseCase
}

func NewRefreshHandler(refreshUseCase *refresh.UseCase) *refreshHandler {
	return &refreshHandler{
		refreshUseCase: refreshUseCase,
	}
}

func (h *refreshHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request refresh.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	response, err := h.refreshUseCase.Login(request)
	switch true {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	case len(response.ValidationErrors) > 0:
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
