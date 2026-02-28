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
	useCase *updatearticle.UseCase
}

func NewUpdateHandler(useCase *updatearticle.UseCase) *updateHandler {
	return &updateHandler{
		useCase: useCase,
	}
}

// @Summary		Update a dashboard article
// @Description	update existing article fields, current user must be author
// @Tags			dashboard articles
// @Accept			json
// @Produce		json
// @Param			article	body	updatearticle.Request	true	"Article update payload"
// @Success		204
// @Failure		400	{object}	map[string]interface{}
// @Failure		404	{object}	map[string]interface{}
// @Failure		500	{object}	map[string]interface{}
// @Router			/dashboard/articles [put]
func (h *updateHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID

	var request updatearticle.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	request.AuthorUUID = userUUID

	response, err := h.useCase.Execute(&request)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	case response != nil && len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
