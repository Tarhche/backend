package article

import (
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	deleteuserarticle "github.com/khanzadimahdi/testproject/application/dashboard/article/deleteUserArticle"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type deleteUserHandler struct {
	useCase *deleteuserarticle.UseCase
}

func NewDeleteUserHandler(useCase *deleteuserarticle.UseCase) *deleteUserHandler {
	return &deleteUserHandler{
		useCase: useCase,
	}
}

// @Summary		Delete own article
// @Description	remove an article the authenticated user wrote, by correlation uuid and language
// @Tags		dashboard articles
// @Accept		json
// @Produce		json
// @Param		correlationUUID	path	string	true	"Article correlation UUID"
// @Param		language_code	path	string	true	"Language code"
// @Success		204
// @Failure		404	{object}	map[string]interface{}
// @Failure		500	{object}	map[string]interface{}
// @Router		/dashboard/my/articles/{correlationUUID}/{language_code} [delete]
func (h *deleteUserHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := &deleteuserarticle.Request{
		CorrelationUUID: r.PathValue("correlationUUID"),
		LanguageCode:    r.PathValue("language_code"),
		AuthorUUID:      auth.FromContext(r.Context()).UUID,
	}

	err := h.useCase.Execute(r.Context(), request)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
