package user

import (
	"encoding/json"
	"errors"
	"net/http"

	createuser "github.com/khanzadimahdi/testproject/application/dashboard/user/createUser"
	"github.com/khanzadimahdi/testproject/domain"
)

type createHandler struct {
	useCase *createuser.UseCase
}

func NewCreateHandler(useCase *createuser.UseCase) *createHandler {
	return &createHandler{
		useCase: useCase,
	}
}

func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request createuser.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	response, err := h.useCase.Execute(&request)

	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	case len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusCreated)
		json.NewEncoder(rw).Encode(response)
	}
}
