package task

import (
	"encoding/json"
	"net/http"
	"strconv"
	"unsafe"

	gettasks "github.com/khanzadimahdi/testproject/application/runner/manager/task/getTasks"
)

type indexHandler struct {
	useCase *gettasks.UseCase
}

func NewIndexHandler(useCase *gettasks.UseCase) *indexHandler {
	return &indexHandler{
		useCase: useCase,
	}
}

// @Summary		List tasks
// @Description	return a page of tasks
// @Tags			runner tasks
// @Accept			json
// @Produce		json
// @Param			page	query		int	false	"Page number"	default(1)
// @Success		200		{object}	gettasks.Response
// @Failure		500		{object}	map[string]interface{}
// @Router			/tasks [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var page uint = 1
	if r.URL.Query().Has("page") {
		parsedPage, err := strconv.ParseUint(r.URL.Query().Get("page"), 10, int(unsafe.Sizeof(page)))
		if err == nil {
			page = uint(parsedPage)
		}
	}

	request := &gettasks.Request{
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
