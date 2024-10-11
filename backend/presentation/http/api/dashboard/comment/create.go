package comment

import (
	"encoding/json"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/comment/createComment"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
)

type createHandler struct {
	useCase    *createComment.UseCase
	authorizer domain.Authorizer
}

func NewCreateHandler(useCase *createComment.UseCase, a domain.Authorizer) *createHandler {
	return &createHandler{
		useCase:    useCase,
		authorizer: a,
	}
}

func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID
	if ok, err := h.authorizer.Authorize(userUUID, permission.CommentsCreate); err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	} else if !ok {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	var request createComment.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	request.AuthorUUID = userUUID

	response, err := h.useCase.Execute(request)

	switch {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	case len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.WriteHeader(http.StatusCreated)
	}
}
