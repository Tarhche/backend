package bookmark

import (
	"encoding/json"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/bookmark/deleteUserBookmark"
)

type deleteUserHandler struct {
	useCase *deleteUserBookmark.UseCase
}

func NewDeleteUserBookmarkHandler(useCase *deleteUserBookmark.UseCase) *deleteUserHandler {
	return &deleteUserHandler{
		useCase: useCase,
	}
}

// @Summary		Delete my bookmark
// @Description	delete a bookmark owned by current user
// @Tags			dashboard bookmarks
// @Accept			json
// @Produce		json
// @Param			body	body	deleteUserBookmark.Request	true	"Bookmark to delete"
// @Success		204
// @Failure		400	{object}	map[string]interface{}
// @Failure		500	{object}	map[string]interface{}
// @Router			/dashboard/my/bookmarks [delete]
func (h *deleteUserHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID

	var request deleteUserBookmark.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	request.OwnerUUID = userUUID

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
