package article

import (
	"encoding/json"
	"errors"
	"net/http"

	createarticle "github.com/khanzadimahdi/testproject.git/application/article/createArticle"
	"github.com/khanzadimahdi/testproject.git/domain"
)

type createHandler struct {
	createArticleUseCase *createarticle.UseCase
}

func NewCreateHandler(createArticleUseCase *createarticle.UseCase) *createHandler {
	return &createHandler{
		createArticleUseCase: createArticleUseCase,
	}
}

func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request createarticle.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	validationErrors, err := h.createArticleUseCase.CreateArticle(request)

	switch true {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case len(validationErrors.ValidationErrors) > 0:
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(validationErrors)
	default:
		rw.WriteHeader(http.StatusCreated)
	}
}
