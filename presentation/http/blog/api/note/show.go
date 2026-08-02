package note

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/localize"
	getnote "github.com/khanzadimahdi/testproject/application/note/getNote"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type showHandler struct {
	useCase *getnote.UseCase
}

func NewShowHandler(useCase *getnote.UseCase) *showHandler {
	return &showHandler{
		useCase: useCase,
	}
}

// @Summary		Show a published note
// @Description	return a published note by its correlation uuid, in the requested language
// @Tags		notes
// @Accept		json
// @Produce		json
// @Param		uuid		path		string	true	"Note correlation UUID"
// @Success		200			{object}	getnote.Response
// @Failure		400			{object}	map[string]interface{}
// @Failure		404			{object}	map[string]interface{}
// @Failure		500			{object}	map[string]interface{}
// @Router		/notes/{uuid} [get]
func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := &getnote.Request{
		CorrelationUUID: r.PathValue("uuid"),
		LanguageCode:    localize.FromContext(r.Context()),
	}

	response, err := h.useCase.Execute(r.Context(), request)

	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	case len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
