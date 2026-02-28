package comment

import (
	"encoding/json"
	"net/http"
	"strconv"
	"unsafe"

	"github.com/khanzadimahdi/testproject/application/dashboard/comment/getComments"
)

type indexHandler struct {
	useCase *getComments.UseCase
}

func NewIndexHandler(useCase *getComments.UseCase) *indexHandler {
	return &indexHandler{
		useCase: useCase,
	}
}

// @Summary		List comments
// @Description	paginated list of comments
// @Tags			dashboard comments
// @Accept			json
// @Produce		json
// @Param			page	query		int	false	"Page"	default(1)
// @Success		200		{object}	getComments.Response
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/comments [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var page uint = 1
	if r.URL.Query().Has("page") {
		parsedPage, err := strconv.ParseUint(r.URL.Query().Get("page"), 10, int(unsafe.Sizeof(page)))
		if err == nil {
			page = uint(parsedPage)
		}
	}

	request := &getComments.Request{
		Page: page,
	}

	response, err := h.useCase.Execute(request)
	switch {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
