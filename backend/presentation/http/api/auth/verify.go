package auth

import (
	"encoding/json"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth/verify"
)

type verifyHandler struct {
	useCase *verify.UseCase
}

func NewVerifyHandler(useCase *verify.UseCase) *verifyHandler {
	return &verifyHandler{
		useCase: useCase,
	}
}

func (h *verifyHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request verify.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	response, err := h.useCase.Execute(request)

	switch {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	case len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
