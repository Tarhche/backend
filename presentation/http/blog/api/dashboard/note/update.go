package note

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	updatenote "github.com/khanzadimahdi/testproject/application/dashboard/note/updateNote"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type updateHandler struct {
	useCase *updatenote.UseCase
	// ownedOnly narrows the update to the current user's own notes, for the
	// route guarded by the self notes permission.
	ownedOnly bool
}

func NewUpdateHandler(useCase *updatenote.UseCase) *updateHandler {
	return &updateHandler{
		useCase: useCase,
	}
}

func NewUpdateUserNoteHandler(useCase *updatenote.UseCase) *updateHandler {
	return &updateHandler{
		useCase:   useCase,
		ownedOnly: true,
	}
}

// @Summary		Update a dashboard note
// @Description	update existing note fields, current user becomes the author
// @Tags			dashboard notes
// @Accept			json
// @Produce		json
// @Param			note	body	updatenote.Request	true	"Note update payload"
// @Success		204
// @Failure		400	{object}	map[string]interface{}
// @Failure		404	{object}	map[string]interface{}
// @Failure		500	{object}	map[string]interface{}
// @Router			/dashboard/notes [put]
// @Router			/dashboard/my/notes [put]
func (h *updateHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID

	var request updatenote.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}
	request.AuthorUUID = userUUID
	if h.ownedOnly {
		request.OwnerUUID = userUUID
	}

	response, err := h.useCase.Execute(r.Context(), &request)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	case response != nil && len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
