package node

import (
	"encoding/json"
	"net/http"
	"strconv"
	"unsafe"

	getnodes "github.com/khanzadimahdi/testproject/application/runner/manager/node/getNodes"
)

type indexHandler struct {
	useCase *getnodes.UseCase
}

func NewIndexHandler(useCase *getnodes.UseCase) *indexHandler {
	return &indexHandler{
		useCase: useCase,
	}
}

// @Summary		List nodes
// @Description	return a page of runner nodes
// @Tags			runner nodes
// @Accept			json
// @Produce		json
// @Param			page	query		int	false	"Page number"	default(1)
// @Success		200		{object}	getnodes.Response
// @Failure		500		{object}	map[string]interface{}
// @Router			/nodes [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var page uint = 1
	if r.URL.Query().Has("page") {
		parsedPage, err := strconv.ParseUint(r.URL.Query().Get("page"), 10, int(unsafe.Sizeof(page)))
		if err == nil {
			page = uint(parsedPage)
		}
	}

	request := &getnodes.Request{
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
