package note

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	getnote "github.com/khanzadimahdi/testproject/application/dashboard/note/getNote"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type showHandler struct {
	useCase *getnote.UseCase
	// ownedOnly narrows the lookup to the current user's own notes, for the
	// route guarded by the self notes permission.
	ownedOnly bool
}

func NewShowHandler(useCase *getnote.UseCase) *showHandler {
	return &showHandler{
		useCase: useCase,
	}
}

func NewShowUserNoteHandler(useCase *getnote.UseCase) *showHandler {
	return &showHandler{
		useCase:   useCase,
		ownedOnly: true,
	}
}

// @Summary		Get dashboard note
// @Description	retrieve one note by correlation uuid and language
// @Tags		dashboard notes
// @Accept		json
// @Produce		json
// @Param		correlationUUID	path		string	true	"Note correlation UUID"
// @Param		language_code	path		string	true	"Language code"
// @Success		200		{object}	getnote.Response
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router		/dashboard/notes/{correlationUUID}/{language_code} [get]
// @Router		/dashboard/my/notes/{correlationUUID}/{language_code} [get]
func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := &getnote.Request{
		CorrelationUUID: r.PathValue("correlationUUID"),
		LanguageCode:    r.PathValue("language_code"),
	}
	if h.ownedOnly {
		request.OwnerUUID = auth.FromContext(r.Context()).UUID
	}

	response, err := h.useCase.Execute(r.Context(), request)

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
