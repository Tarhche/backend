package contact

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/contact/markAsRead"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type markAsReadHandler struct {
	useCase *markAsRead.UseCase
}

func NewMarkAsReadHandler(useCase *markAsRead.UseCase) *markAsReadHandler {
	return &markAsReadHandler{
		useCase: useCase,
	}
}

// @Summary		Mark a contact-us message as read
// @Description	toggle the read state of a contact-us message; the read time is stamped by the server
// @Tags			dashboard contact-us
// @Accept			json
// @Produce		json
// @Param			uuid	path		string					true	"Message UUID"
// @Param			body	body		markAsRead.Request		true	"Read state"
// @Success		200		{object}	markAsRead.Response
// @Failure		400		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/contact-us/{uuid}/read [put]
func (h *markAsReadHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request markAsRead.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	request.MessageUUID = r.PathValue("uuid")

	response, err := h.useCase.Execute(r.Context(), &request)
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
