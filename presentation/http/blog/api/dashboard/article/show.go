package article

import (
	"encoding/json"
	"errors"
	"net/http"

	getarticle "github.com/khanzadimahdi/testproject/application/dashboard/article/getArticle"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type showHandler struct {
	useCase *getarticle.UseCase
}

func NewShowHandler(useCase *getarticle.UseCase) *showHandler {
	return &showHandler{
		useCase: useCase,
	}
}

// @Summary		Get dashboard article
// @Description	retrieve one article by correlation uuid and language
// @Tags		dashboard articles
// @Accept		json
// @Produce		json
// @Param		correlationUUID	path		string	true	"Article correlation UUID"
// @Param		language_code	path		string	true	"Language code"
// @Success		200		{object}	getarticle.Response
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router		/dashboard/articles/{correlationUUID}/{language_code} [get]
func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := &getarticle.Request{
		CorrelationUUID: r.PathValue("correlationUUID"),
		LanguageCode:    r.PathValue("language_code"),
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
