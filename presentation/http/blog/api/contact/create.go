package contact

import (
	"encoding/json"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/contact/createMessage"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type createHandler struct {
	useCase *createMessage.UseCase
}

func NewCreateHandler(useCase *createMessage.UseCase) *createHandler {
	return &createHandler{
		useCase: useCase,
	}
}

// @Summary		Send a contact-us message
// @Description	send a message to the site owners; the sender needs no account
// @Tags			contact-us
// @Accept			json
// @Produce		json
// @Param			message	body		createMessage.Request	true	"Contact-us payload"
// @Success		201		{object}	map[string]interface{}
// @Failure		400		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/contact-us [post]
func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request createMessage.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	response, err := h.useCase.Execute(r.Context(), &request)

	switch {
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	case response != nil && len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.WriteHeader(http.StatusCreated)
	}
}
