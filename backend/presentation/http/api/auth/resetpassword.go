package auth

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth/resetpassword"
	"github.com/khanzadimahdi/testproject/domain"
)

type resetPasswordHandler struct {
	useCase *resetpassword.UseCase
}

func NewResetPasswordHandler(useCase *resetpassword.UseCase) *resetPasswordHandler {
	return &resetPasswordHandler{
		useCase: useCase,
	}
}

func (h *resetPasswordHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request resetpassword.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	response, err := h.useCase.ResetPassword(request)

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
		rw.WriteHeader(http.StatusNoContent)
	}
}
