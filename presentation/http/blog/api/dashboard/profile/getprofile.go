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

// @Summary		Get profile
// @Description	retrieve profile for current user
// @Tags			dashboard profile
// @Accept			json
// @Produce		json
// @Success		200	{object}	getprofile.Response
// @Failure		404	{object}	map[string]interface{}
// @Failure		500	{object}	map[string]interface{}
// @Router			/dashboard/profile [get]
func (h *getProfileHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID

	response, err := h.useCase.Execute(userUUID)

	switch {
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
