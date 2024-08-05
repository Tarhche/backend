package comment

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/comment/createComment"
	"github.com/khanzadimahdi/testproject/domain"
)

type createHandler struct {
	createCommentUseCase *createComment.UseCase
}

func NewCreateHandler(createCommentUseCase *createComment.UseCase) *createHandler {
	return &createHandler{
		createCommentUseCase: createCommentUseCase,
	}
}

func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request createComment.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	request.AuthorUUID = auth.FromContext(r.Context()).UUID

	response, err := h.createCommentUseCase.Execute(request)

	switch true {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	case len(response.ValidationErrors) > 0:
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.WriteHeader(http.StatusCreated)
	}
}
