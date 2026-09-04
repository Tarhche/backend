package article

import (
	"encoding/json"
	"net/http"
	"strconv"
	"unsafe"

	"github.com/khanzadimahdi/testproject/application/auth"
	getuserarticles "github.com/khanzadimahdi/testproject/application/dashboard/article/getUserArticles"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type indexUserHandler struct {
	useCase *getuserarticles.UseCase
}

func NewIndexUserHandler(useCase *getuserarticles.UseCase) *indexUserHandler {
	return &indexUserHandler{
		useCase: useCase,
	}
}

// @Summary		List own articles
// @Description	page through the articles the authenticated user wrote, grouped by correlation uuid
// @Tags		dashboard articles
// @Accept		json
// @Produce		json
// @Param		page		query		int		false	"Page"	default(1)
// @Success		200		{object}	getuserarticles.Response
// @Failure		500		{object}	map[string]interface{}
// @Router		/dashboard/my/articles [get]
func (h *indexUserHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var page uint = 1
	if r.URL.Query().Has("page") {
		parsedPage, err := strconv.ParseUint(r.URL.Query().Get("page"), 10, int(unsafe.Sizeof(page)))
		if err == nil {
			page = uint(parsedPage)
		}
	}

	request := &getuserarticles.Request{
		Page:       page,
		AuthorUUID: auth.FromContext(r.Context()).UUID,
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
