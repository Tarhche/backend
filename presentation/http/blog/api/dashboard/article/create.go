package article

import (
	"encoding/json"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	createarticle "github.com/khanzadimahdi/testproject/application/dashboard/article/createArticle"
)

type createHandler struct {
	useCase *createarticle.UseCase
}

func NewCreateHandler(useCase *createarticle.UseCase) *createHandler {
	return &createHandler{
		useCase: useCase,
	}
}

// @Summary		Create dashboard article
// @Description	create a new article in dashboard as current user
// @Tags			dashboard articles
// @Accept			json
// @Produce		json
// @Param			article	body		createarticle.Request	true	"Article data"
// @Success		201		{object}	createarticle.Response
// @Failure		400		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/articles [post]
func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID

	var request createarticle.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	request.AuthorUUID = userUUID

	response, err := h.useCase.Execute(&request)

	switch {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	case response != nil && len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusCreated)
		json.NewEncoder(rw).Encode(response)
	}
}
