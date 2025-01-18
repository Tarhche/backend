package profile

import (
	"encoding/json"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	getroles "github.com/khanzadimahdi/testproject/application/dashboard/profile/getRoles"
)

type getRolesHandler struct {
	useCase *getroles.UseCase
}

func NewGetRolesHandler(useCase *getroles.UseCase) *getRolesHandler {
	return &getRolesHandler{
		useCase: useCase,
	}
}

func (h *getRolesHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID

	response, err := h.useCase.Execute(userUUID)
	switch {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
