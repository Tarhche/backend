package bookmark

import (
	"encoding/json"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/bookmark/bookmarkExists"
)

type existsHandler struct {
	useCase *bookmarkExists.UseCase
}

func NewExistsHandler(useCase *bookmarkExists.UseCase) *existsHandler {
	return &existsHandler{
		useCase: useCase,
	}
}

// @Summary		Check bookmark existence
// @Description	return whether a bookmark exists for the given article
// @Tags			bookmarks
// @Accept			json
// @Produce		json
// @Param			body	body		bookmarkExists.Request	true	"Bookmark existence request"
// @Success		200		{object}	bookmarkExists.Response
// @Failure		400		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/bookmarks/exists [post]
func (h *existsHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request bookmarkExists.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	request.OwnerUUID = auth.FromContext(r.Context()).UUID

	response, err := h.useCase.Execute(&request)

	switch {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	case len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
