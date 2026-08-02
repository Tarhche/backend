package contact

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/contact/getMessage"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type showHandler struct {
	useCase *getMessage.UseCase
}

func NewShowHandler(useCase *getMessage.UseCase) *showHandler {
	return &showHandler{
		useCase: useCase,
	}
}

// @Summary		Get a contact-us message
// @Description	retrieve a contact-us message by UUID
// @Tags			dashboard contact-us
// @Accept			json
// @Produce		json
// @Param			uuid	path		string	true	"Message UUID"
// @Success		200		{object}	getMessage.Response
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/contact-us/{uuid} [get]
func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	UUID := r.PathValue("uuid")

	response, err := h.useCase.Execute(r.Context(), UUID)

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
