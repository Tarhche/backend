package note

import (
	"encoding/json"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	createnote "github.com/khanzadimahdi/testproject/application/dashboard/note/createNote"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type createHandler struct {
	useCase *createnote.UseCase
}

func NewCreateHandler(useCase *createnote.UseCase) *createHandler {
	return &createHandler{
		useCase: useCase,
	}
}

// @Summary		Create dashboard note
// @Description	create a new note in dashboard as current user
// @Tags			dashboard notes
// @Accept			json
// @Produce		json
// @Param			note	body		createnote.Request	true	"Note data"
// @Success		201		{object}	createnote.Response
// @Failure		400		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/notes [post]
// @Router			/dashboard/my/notes [post]
func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID

	var request createnote.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	request.AuthorUUID = userUUID

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
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusCreated)
		json.NewEncoder(rw).Encode(response)
	}
}
