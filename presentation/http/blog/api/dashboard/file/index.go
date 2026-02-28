package file

import (
	"encoding/json"
	"net/http"
	"strconv"
	"unsafe"

	getfiles "github.com/khanzadimahdi/testproject/application/dashboard/file/getFiles"
)

type indexHandler struct {
	useCase *getfiles.UseCase
}

func NewIndexHandler(useCase *getfiles.UseCase) *indexHandler {
	return &indexHandler{
		useCase: useCase,
	}
}

// @Summary		List files
// @Description	paginated list of all files
// @Tags			dashboard files
// @Accept			json
// @Produce		json
// @Param			page	query		int	false	"Page"	default(1)
// @Success		200		{object}	getfiles.Response
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/files [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var page uint = 1
	if r.URL.Query().Has("page") {
		parsedPage, err := strconv.ParseUint(r.URL.Query().Get("page"), 10, int(unsafe.Sizeof(page)))
		if err == nil {
			page = uint(parsedPage)
		}
	}

	request := &getfiles.Request{
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
