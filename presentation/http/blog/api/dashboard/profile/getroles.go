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

// @Summary		Get user roles
// @Description	returns roles for current user
// @Tags			dashboard profile
// @Accept			json
// @Produce		json
// @Success		200	{object}	getroles.Response
// @Failure		500	{object}	map[string]interface{}
// @Router			/dashboard/profile/roles [get]
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
