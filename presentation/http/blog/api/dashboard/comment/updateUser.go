package comment

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/comment/updateUserComment"
	"github.com/khanzadimahdi/testproject/domain"
)

type updateUserHandler struct {
	useCase *updateUserComment.UseCase
}

func NewUpdateUserCommentHandler(useCase *updateUserComment.UseCase) *updateUserHandler {
	return &updateUserHandler{
		useCase: useCase,
	}
}

// @Summary		Update other user's comment
// @Description	admin action to edit a user's comment
// @Tags			dashboard comments
// @Accept			json
// @Produce		json
// @Param			body	body	updateUserComment.Request	true	"Comment update"
// @Success		204
// @Failure		400	{object}	map[string]interface{}
// @Failure		404	{object}	map[string]interface{}
// @Failure		500	{object}	map[string]interface{}
// @Router			/dashboard/comments/user [put]
func (h *updateUserHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID

	var request updateUserComment.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	request.UserUUID = userUUID

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
