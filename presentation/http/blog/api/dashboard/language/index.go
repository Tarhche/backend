package language

import (
	"encoding/json"
	"net/http"
	"strconv"
	"unsafe"

	getlanguages "github.com/khanzadimahdi/testproject/application/dashboard/language/getLanguages"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type indexHandler struct {
	useCase *getlanguages.UseCase
}

func NewIndexHandler(useCase *getlanguages.UseCase) *indexHandler {
	return &indexHandler{
		useCase: useCase,
	}
}

// @Summary		List languages
// @Description	paginated list of languages
// @Tags			dashboard languages
// @Accept			json
// @Produce		json
// @Param			page	query		int	false	"Page"	default(1)
// @Success		200		{object}	getlanguages.Response
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/languages [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var page uint = 1
	if r.URL.Query().Has("page") {
		parsedPage, err := strconv.ParseUint(r.URL.Query().Get("page"), 10, int(unsafe.Sizeof(page)))
		if err == nil {
			page = uint(parsedPage)
		}
	}

	request := &getlanguages.Request{
		Page: page,
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
