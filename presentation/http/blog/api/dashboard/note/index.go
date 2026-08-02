package note

import (
	"encoding/json"
	"net/http"
	"strconv"
	"unsafe"

	"github.com/khanzadimahdi/testproject/application/auth"
	getnotes "github.com/khanzadimahdi/testproject/application/dashboard/note/getNotes"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type indexHandler struct {
	useCase *getnotes.UseCase
	// ownedOnly narrows the listing to the current user's own notes, for the
	// route guarded by the self notes permission.
	ownedOnly bool
}

func NewIndexHandler(useCase *getnotes.UseCase) *indexHandler {
	return &indexHandler{
		useCase: useCase,
	}
}

func NewIndexUserNotesHandler(useCase *getnotes.UseCase) *indexHandler {
	return &indexHandler{
		useCase:   useCase,
		ownedOnly: true,
	}
}

// @Summary		List dashboard notes
// @Description	page through notes in dashboard, grouped by correlation uuid
// @Tags		dashboard notes
// @Accept		json
// @Produce		json
// @Param		page		query		int		false	"Page"	default(1)
// @Success		200		{object}	getnotes.Response
// @Failure		500		{object}	map[string]interface{}
// @Router		/dashboard/notes [get]
// @Router		/dashboard/my/notes [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var page uint = 1
	if r.URL.Query().Has("page") {
		parsedPage, err := strconv.ParseUint(r.URL.Query().Get("page"), 10, int(unsafe.Sizeof(page)))
		if err == nil {
			page = uint(parsedPage)
		}
	}

	request := &getnotes.Request{
		Page: page,
	}
	if h.ownedOnly {
		request.AuthorUUID = auth.FromContext(r.Context()).UUID
	}

	response, err := h.useCase.Execute(r.Context(), request)
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
