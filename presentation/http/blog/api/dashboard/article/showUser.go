package article

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	getuserarticle "github.com/khanzadimahdi/testproject/application/dashboard/article/getUserArticle"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type showUserHandler struct {
	useCase *getuserarticle.UseCase
}

func NewShowUserHandler(useCase *getuserarticle.UseCase) *showUserHandler {
	return &showUserHandler{
		useCase: useCase,
	}
}

// @Summary		Show own article
// @Description	an article the authenticated user wrote, by correlation uuid and language
// @Tags		dashboard articles
// @Accept		json
// @Produce		json
// @Param		correlationUUID	path		string	true	"Article correlation UUID"
// @Param		language_code	path		string	true	"Language code"
// @Success		200		{object}	getuserarticle.Response
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router		/dashboard/my/articles/{correlationUUID}/{language_code} [get]
func (h *showUserHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := &getuserarticle.Request{
		CorrelationUUID: r.PathValue("correlationUUID"),
		LanguageCode:    r.PathValue("language_code"),
		AuthorUUID:      auth.FromContext(r.Context()).UUID,
	}

	response, err := h.useCase.Execute(r.Context(), request)

	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
