package auth

import (
	"encoding/json"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth/login"
)

type loginHandler struct {
	useCase *login.UseCase
}

func NewLoginHandler(useCase *login.UseCase) *loginHandler {
	return &loginHandler{
		useCase: useCase,
	}
}

// @Summary		Login
// @Description	obtain authentication tokens
// @Tags			auth
// @Accept			json
// @Produce		json
// @Param			body	body		login.Request	true	"Credentials"
// @Success		200		{object}	login.Response
// @Failure		400		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/auth/login [post]
func (h *loginHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request login.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	response, err := h.useCase.Execute(&request)
	switch {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	case response != nil && len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
