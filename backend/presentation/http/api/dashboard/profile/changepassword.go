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

func (h *changePasswordHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request changepassword.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	request.UserUUID = auth.FromContext(r.Context()).UUID

	response, err := h.userCase.ChangePassword(request)

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
