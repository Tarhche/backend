package profile

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/profile/changepassword"
	"github.com/khanzadimahdi/testproject/domain"
)

type changePasswordHandler struct {
	userCase *changepassword.UseCase
}

func NewChangePasswordHandler(userCase *changepassword.UseCase) *changePasswordHandler {
	return &changePasswordHandler{
		userCase: userCase,
	}
}

// @Summary		Change user password
// @Description	update password for current authenticated user
// @Tags			dashboard profile
// @Accept			json
// @Produce		json
// @Param			body	body	changepassword.Request	true	"Password change"
// @Success		204
// @Failure		400	{object}	map[string]interface{}
// @Failure		404	{object}	map[string]interface{}
// @Failure		500	{object}	map[string]interface{}
// @Router			/dashboard/profile/password [put]
func (h *changePasswordHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request changepassword.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	request.UserUUID = auth.FromContext(r.Context()).UUID

	response, err := h.userCase.Execute(&request)

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
