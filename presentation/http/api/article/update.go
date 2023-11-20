package article

import (
	"encoding/json"
	"errors"
	"net/http"

	updatearticle "github.com/khanzadimahdi/testproject.git/application/article/updateArticle"
	"github.com/khanzadimahdi/testproject.git/domain"
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

	validationErrors, err := h.updateArticleUseCase.UpdateArticle(request)
	switch true {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	case len(validationErrors.ValidationErrors) > 0:
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(validationErrors)
	default:
		rw.WriteHeader(http.StatusOK)
	}
}
