package permission

import (
	"encoding/json"
	"net/http"

	getpermissions "github.com/khanzadimahdi/testproject/application/dashboard/permission/getPermissions"
)

type indexHandler struct {
	useCase *getpermissions.UseCase
}

func NewIndexHandler(useCase *getpermissions.UseCase) *indexHandler {
	return &indexHandler{
		useCase: useCase,
	}
}

// @Summary		List permissions
// @Description	retrieve all permissions
// @Tags			dashboard permissions
// @Accept			json
// @Produce		json
// @Success		200	{object}	getpermissions.Response
// @Failure		500	{object}	map[string]interface{}
// @Router			/dashboard/permissions [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute()
	switch {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
