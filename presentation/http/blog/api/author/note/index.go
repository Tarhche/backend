package note

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"unsafe"

	"github.com/gofrs/uuid/v5"

	"github.com/khanzadimahdi/testproject/application/localize"
	getNotesByAuthor "github.com/khanzadimahdi/testproject/application/note/getNotesByAuthor"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type indexHandler struct {
	useCase *getNotesByAuthor.UseCase
}

func NewIndexHandler(useCase *getNotesByAuthor.UseCase) *indexHandler {
	return &indexHandler{
		useCase: useCase,
	}
}

// @Summary		List notes by author
// @Description	return a page of the most recent published notes for the given author identity (UUID or username). Responds 404 when the author keeps their notes private.
// @Tags		authors
// @Accept		json
// @Produce		json
// @Param		identity	    path		string	true	"Author UUID or username"
// @Param		page		    query		int		false	"Page number"	default(1)
// @Success		200			{object}	getNotesByAuthor.Response
// @Failure		400			{object}	map[string]interface{}
// @Failure		404			{object}	map[string]interface{}
// @Failure		500			{object}	map[string]interface{}
// @Router			/authors/{identity}/notes [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var page uint = 1
	if r.URL.Query().Has("page") {
		parsedPage, err := strconv.ParseUint(r.URL.Query().Get("page"), 10, int(unsafe.Sizeof(page)))
		if err == nil {
			page = uint(parsedPage)
		}
	}

	request := &getNotesByAuthor.Request{
		Page:         page,
		LanguageCode: localize.FromContext(r.Context()),
	}

	identity := r.PathValue("identity")
	if _, err := uuid.FromString(identity); err == nil {
		request.AuthorUUID = identity
	} else {
		request.Username = identity
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
