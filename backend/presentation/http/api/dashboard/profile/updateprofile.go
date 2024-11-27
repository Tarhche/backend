package profile

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/profile/updateprofile"
	"github.com/khanzadimahdi/testproject/domain"
)

type updateProfileHandler struct {
	useCase *updateprofile.UseCase
}

func NewUpdateProfileHandler(useCase *updateprofile.UseCase) *updateProfileHandler {
	return &updateProfileHandler{
		useCase: useCase,
	}
}

func (h *updateProfileHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request updateprofile.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	request.UserUUID = auth.FromContext(r.Context()).UUID

	response, err := h.useCase.Execute(&request)

	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	case response != nil && len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
