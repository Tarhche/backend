package ingress

import (
	"encoding/json"
	"net/http"

	getRunners "github.com/khanzadimahdi/testproject/application/runner/ingress/getRunners"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type indexHandler struct {
	useCase *getRunners.UseCase
}

var _ http.Handler = &indexHandler{}

func NewIndexHandler(useCase *getRunners.UseCase) *indexHandler {
	return &indexHandler{
		useCase: useCase,
	}
}

// @Summary		List runners
// @Description	return the runners the ingress can currently route to
// @Tags			runner ingress
// @Produce		json
// @Success		200	{object}	getRunners.Response
// @Failure		500	{object}	map[string]interface{}
// @Router			/api/runners [get]
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
