package bookmark

import (
	"encoding/json"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/bookmark/updateBookmark"
)

type updateHandler struct {
	useCase *updateBookmark.UseCase
}

func NewUpdateHandler(useCase *updateBookmark.UseCase) *updateHandler {
	return &updateHandler{
		useCase: useCase,
	}
}

// @Summary		Create or update bookmark
// @Description	create or update an article bookmark for authenticated user
// @Tags			bookmarks
// @Accept			json
// @Produce		json
// @Param			body	body		updateBookmark.Request	true	"Bookmark request"
// @Success		201		{object}	map[string]interface{}
// @Failure		400		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/bookmarks [put]
func (h *updateHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request updateBookmark.Request
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
		rw.WriteHeader(http.StatusCreated)
	}
}
