package comment

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/comment/deleteComment"
)

type deleteHandler struct {
	useCase *deleteComment.UseCase
}

func NewDeleteHandler(useCase *deleteComment.UseCase) *deleteHandler {
	return &deleteHandler{
		useCase: useCase,
	}
}

// @Summary		Delete comment
// @Description	delete comment by UUID
// @Tags			dashboard comments
// @Accept			json
// @Produce		json
// @Param			uuid	path	string	true	"Comment UUID"
// @Success		204
// @Failure		500	{object}	map[string]interface{}
// @Router			/dashboard/comments/{uuid} [delete]
func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	UUID := r.PathValue("uuid")

	request := deleteComment.Request{
		CommentUUID: UUID,
	}

	err := h.useCase.Execute(&request)
	switch {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
