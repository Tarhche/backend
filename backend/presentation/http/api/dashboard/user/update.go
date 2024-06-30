package user

import (
	"encoding/json"
	"errors"
	updateuser "github.com/khanzadimahdi/testproject/application/dashboard/user/updateUser"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
)

type updateHandler struct {
	useCase *updateuser.UseCase
}

func NewUpdateHandler(useCase *updateuser.UseCase) *updateHandler {
	return &updateHandler{
		useCase: useCase,
	}
}

func (h *updateHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request updateuser.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	request.UserUUID = auth.FromContext(r.Context()).UUID

	response, err := h.useCase.UpdateUser(request)

	switch true {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	case len(response.ValidationErrors) > 0:
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
