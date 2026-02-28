package article

import (
	"encoding/json"
	"errors"
	"net/http"

	getarticle "github.com/khanzadimahdi/testproject/application/article/getArticle"
	"github.com/khanzadimahdi/testproject/domain"
)

type showHandler struct {
	useCase *getarticle.UseCase
}

func NewShowHandler(useCase *getarticle.UseCase) *showHandler {
	return &showHandler{
		useCase: useCase,
	}
}

// @Summary		Retrieve a single article
// @Description	get one published article by UUID
// @Tags			articles
// @Accept			json
// @Produce		json
// @Param			uuid	path		string	true	"Article UUID"
// @Success		200		{object}	getarticle.Response
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/articles/{uuid} [get]
func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	UUID := r.PathValue("uuid")

	response, err := h.useCase.Execute(UUID)

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
