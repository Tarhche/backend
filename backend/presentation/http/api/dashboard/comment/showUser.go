package comment

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/julienschmidt/httprouter"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/comment/getUserComment"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
)

type showUserHandler struct {
	useCase    *getUserComment.UseCase
	authorizer domain.Authorizer
}

func NewShowUserCommentHandler(useCase *getUserComment.UseCase, a domain.Authorizer) *showUserHandler {
	return &showUserHandler{
		useCase:    useCase,
		authorizer: a,
	}
}

func (h *showUserHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID
	if ok, err := h.authorizer.Authorize(userUUID, permission.SelfCommentsShow); err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	} else if !ok {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	UUID := httprouter.ParamsFromContext(r.Context()).ByName("uuid")

	response, err := h.useCase.Execute(UUID, userUUID)

	switch true {
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
