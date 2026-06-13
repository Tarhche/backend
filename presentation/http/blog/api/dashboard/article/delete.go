package article

import (
	"net/http"

	deletearticle "github.com/khanzadimahdi/testproject/application/dashboard/article/deleteArticle"
)

type deleteHandler struct {
	useCase *deletearticle.UseCase
}

func NewDeleteHandler(useCase *deletearticle.UseCase) *deleteHandler {
	return &deleteHandler{
		useCase: useCase,
	}
}

// @Summary		Delete dashboard article
// @Description	remove an article by correlation uuid and language
// @Tags		dashboard articles
// @Accept		json
// @Produce		json
// @Param		correlationUUID	path	string	true	"Article correlation UUID"
// @Param		language_code	path	string	true	"Language code"
// @Success		204
// @Failure		500	{object}	map[string]interface{}
// @Router		/dashboard/articles/{correlationUUID}/{language_code} [delete]
func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := &deletearticle.Request{
		CorrelationUUID: r.PathValue("correlationUUID"),
		LanguageCode:    r.PathValue("language_code"),
	}

	err := h.useCase.Execute(request)
	switch {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
