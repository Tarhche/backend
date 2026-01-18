package comment

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/comment/getUserComment"
	"github.com/khanzadimahdi/testproject/domain"
)

type showUserHandler struct {
	useCase *getUserComment.UseCase
}

func NewShowUserCommentHandler(useCase *getUserComment.UseCase) *showUserHandler {
	return &showUserHandler{
		useCase: useCase,
	}
}

func (h *showUserHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID

	UUID := r.PathValue("uuid")

	response, err := h.useCase.Execute(UUID, userUUID)

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
