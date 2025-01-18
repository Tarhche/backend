package bookmark

import (
	"encoding/json"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/bookmark/deleteUserBookmark"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
)

type deleteUserHandler struct {
	useCase    *deleteUserBookmark.UseCase
	authorizer domain.Authorizer
}

func NewDeleteUserBookmarkHandler(useCase *deleteUserBookmark.UseCase, a domain.Authorizer) *deleteUserHandler {
	return &deleteUserHandler{
		useCase:    useCase,
		authorizer: a,
	}
}

func (h *deleteUserHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID
	if ok, err := h.authorizer.Authorize(userUUID, permission.SelfBookmarksDelete); err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	} else if !ok {
		rw.WriteHeader(http.StatusForbidden)
		return
	}

	var request deleteUserBookmark.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	request.OwnerUUID = auth.FromContext(r.Context()).UUID

	response, err := h.useCase.Execute(&request)
	switch {
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
