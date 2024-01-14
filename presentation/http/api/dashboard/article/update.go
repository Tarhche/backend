package article

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	updatearticle "github.com/khanzadimahdi/testproject/application/dashboard/article/updateArticle"
	"github.com/khanzadimahdi/testproject/domain"
)

type updateHandler struct {
	updateArticleUseCase *updatearticle.UseCase
}

func NewUpdateHandler(updateArticleUseCase *updatearticle.UseCase) *updateHandler {
	return &updateHandler{
		updateArticleUseCase: updateArticleUseCase,
	}
}

func (h *updateHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request updatearticle.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	request.AuthorUUID = auth.FromContext(r.Context()).UUID

	response, err := h.updateArticleUseCase.UpdateArticle(request)
	switch true {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	case len(response.ValidationErrors) > 0:
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.WriteHeader(http.StatusOK)
	}
}
