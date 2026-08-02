package hashtag

import (
	"encoding/json"
	"net/http"
	"strconv"
	"unsafe"

	getContentsByHashtag "github.com/khanzadimahdi/testproject/application/hashtag/getContentsByHashtag"
	"github.com/khanzadimahdi/testproject/application/localize"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type showHandler struct {
	useCase *getContentsByHashtag.UseCase
}

func NewShowHandler(useCase *getContentsByHashtag.UseCase) *showHandler {
	return &showHandler{
		useCase: useCase,
	}
}

// @Summary		List contents by hashtag
// @Description	return a page of the most recent published articles or notes with the given hashtag. Each type is paginated on its own count; omitting `type` selects articles, or notes when the hashtag has no articles.
// @Tags		hashtags
// @Accept		json
// @Produce		json
// @Param		hashtag			path		string	true	"Hashtag"
// @Param		type		    query		string	false	"Content type"	Enums(article, note)
// @Param		page		    query		int		false	"Page number"	default(1)
// @Success		200		{object}	getContentsByHashtag.Response
// @Failure		400		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router		/hashtags/{hashtag} [get]
func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var page uint = 1
	if r.URL.Query().Has("page") {
		parsedPage, err := strconv.ParseUint(r.URL.Query().Get("page"), 10, int(unsafe.Sizeof(page)))
		if err == nil {
			page = uint(parsedPage)
		}
	}

	hashtag := r.PathValue("hashtag")

	request := &getContentsByHashtag.Request{
		Page:         page,
		Hashtag:      hashtag,
		Type:         r.URL.Query().Get("type"),
		LanguageCode: localize.FromContext(r.Context()),
	}

	response, err := h.useCase.Execute(r.Context(), request)

	switch {
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
