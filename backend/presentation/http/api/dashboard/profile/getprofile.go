package profile

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/profile/getprofile"
	"github.com/khanzadimahdi/testproject/domain"
)

type getProfileHandler struct {
	useCase *getprofile.UseCase
}

func NewGetProfileHandler(useCase *getprofile.UseCase) *getProfileHandler {
	return &getProfileHandler{
		useCase: useCase,
	}
}

func (h *getProfileHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID

	response, err := h.useCase.Execute(userUUID)

	switch true {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
