package comment

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/comment/deleteUserComment"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type deleteUserHandler struct {
	useCase *deleteUserComment.UseCase
}

func NewDeleteUserCommentHandler(useCase *deleteUserComment.UseCase) *deleteUserHandler {
	return &deleteUserHandler{
		useCase: useCase,
	}
}

// @Summary		Delete comment (user scoped)
// @Description	remove a comment belonging to specified user
// @Tags			dashboard comments
// @Accept			json
// @Produce		json
// @Param			uuid		path	string	true	"Comment UUID"
// @Param			user_uuid	query	string	true	"User UUID"
// @Success		204
// @Failure		500	{object}	map[string]interface{}
// @Router			/dashboard/comments/user/{uuid} [delete]
func (h *deleteUserHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID

	UUID := r.PathValue("uuid")

	request := deleteUserComment.Request{
		CommentUUID: UUID,
		UserUUID:    userUUID,
	}

	err := h.useCase.Execute(r.Context(), &request)
	switch {
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
