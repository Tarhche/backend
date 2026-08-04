package contact

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/contact/deleteMessage"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type deleteHandler struct {
	useCase *deleteMessage.UseCase
}

func NewDeleteHandler(useCase *deleteMessage.UseCase) *deleteHandler {
	return &deleteHandler{
		useCase: useCase,
	}
}

// @Summary		Delete a contact-us message
// @Description	delete a contact-us message by UUID
// @Tags			dashboard contact-us
// @Accept			json
// @Produce		json
// @Param			uuid	path	string	true	"Message UUID"
// @Success		204
// @Failure		500	{object}	map[string]interface{}
// @Router			/dashboard/contact-us/{uuid} [delete]
func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	UUID := r.PathValue("uuid")

	request := deleteMessage.Request{
		MessageUUID: UUID,
	}

	err := h.useCase.Execute(r.Context(), &request)
	switch {
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
