package auth

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject.git/application/auth/login"
	"github.com/khanzadimahdi/testproject.git/domain"
)

type loginHandler struct {
	loginUseCase *login.UseCase
}

func NewLoginHandler(loginUseCase *login.UseCase) *loginHandler {
	return &loginHandler{
		loginUseCase: loginUseCase,
	}
}

func (h *loginHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request login.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	response, err := h.loginUseCase.Login(request)
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
