package auth

import (
	"encoding/json"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth/forgetpassword"
)

type forgetPasswordHandler struct {
	useCase *forgetpassword.UseCase
}

func NewForgetPasswordHandler(useCase *forgetpassword.UseCase) *forgetPasswordHandler {
	return &forgetPasswordHandler{
		useCase: useCase,
	}
}

// @Summary		Forgot password
// @Description	request a password reset email
// @Tags			auth
// @Accept			json
// @Produce		json
// @Param			body	body		forgetpassword.Request	true	"Email address"
// @Success		204		{object}	map[string]interface{}
// @Failure		400		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/auth/password/forget [post]
func (h *forgetPasswordHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request forgetpassword.Request
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
		rw.WriteHeader(http.StatusNoContent)
	}
}
