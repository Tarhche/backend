package language

import (
	"encoding/json"
	"net/http"

	getlanguages "github.com/khanzadimahdi/testproject/application/language/getLanguages"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type indexHandler struct {
	useCase *getlanguages.UseCase
}

func NewIndexHandler(useCase *getlanguages.UseCase) *indexHandler {
	return &indexHandler{
		useCase: useCase,
	}
}

// @Summary		List languages
// @Description	retrieve all available languages
// @Tags			languages
// @Accept			json
// @Produce		json
// @Success		200	{object}	getlanguages.Response
// @Failure		500	{object}	map[string]interface{}
// @Router			/languages [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context())
	switch {
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
