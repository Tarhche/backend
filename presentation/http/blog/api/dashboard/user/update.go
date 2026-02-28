package user

import (
	"encoding/json"
	"errors"
	"net/http"

	updateuser "github.com/khanzadimahdi/testproject/application/dashboard/user/updateUser"
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

// @Summary		Update user
// @Description	update a user's data
// @Tags			dashboard users
// @Accept			json
// @Produce		json
// @Param			body	body	updateuser.Request	true	"User update"
// @Success		204
// @Failure		400	{object}	map[string]interface{}
// @Failure		404	{object}	map[string]interface{}
// @Failure		500	{object}	map[string]interface{}
// @Router			/dashboard/users [put]
func (h *updateHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request updateuser.Request
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
	case response != nil && len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
