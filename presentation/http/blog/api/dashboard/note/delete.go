package note

import (
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	deletenote "github.com/khanzadimahdi/testproject/application/dashboard/note/deleteNote"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type deleteHandler struct {
	useCase *deletenote.UseCase
	// ownedOnly narrows the deletion to the current user's own notes, for the
	// route guarded by the self notes permission.
	ownedOnly bool
}

func NewDeleteHandler(useCase *deletenote.UseCase) *deleteHandler {
	return &deleteHandler{
		useCase: useCase,
	}
}

func NewDeleteUserNoteHandler(useCase *deletenote.UseCase) *deleteHandler {
	return &deleteHandler{
		useCase:   useCase,
		ownedOnly: true,
	}
}

// @Summary		Delete dashboard note
// @Description	remove a note by correlation uuid and language
// @Tags		dashboard notes
// @Accept		json
// @Produce		json
// @Param		correlationUUID	path	string	true	"Note correlation UUID"
// @Param		language_code	path	string	true	"Language code"
// @Success		204
// @Failure		404	{object}	map[string]interface{}
// @Failure		500	{object}	map[string]interface{}
// @Router		/dashboard/notes/{correlationUUID}/{language_code} [delete]
// @Router		/dashboard/my/notes/{correlationUUID}/{language_code} [delete]
func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := &deletenote.Request{
		CorrelationUUID: r.PathValue("correlationUUID"),
		LanguageCode:    r.PathValue("language_code"),
	}
	if h.ownedOnly {
		request.OwnerUUID = auth.FromContext(r.Context()).UUID
	}

	err := h.useCase.Execute(r.Context(), request)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
