package role

import (
	"encoding/json"
	"net/http"
	"strconv"
	"unsafe"

	getroles "github.com/khanzadimahdi/testproject/application/dashboard/role/getRoles"
)

type indexHandler struct {
	useCase *getroles.UseCase
}

func NewIndexHandler(useCase *getroles.UseCase) *indexHandler {
	return &indexHandler{
		useCase: useCase,
	}
}

// @Summary		List roles
// @Description	paginated list of roles
// @Tags			dashboard roles
// @Accept			json
// @Produce		json
// @Param			page	query		int	false	"Page"	default(1)
// @Success		200		{object}	getroles.Response
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/roles [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var page uint = 1
	if r.URL.Query().Has("page") {
		parsedPage, err := strconv.ParseUint(r.URL.Query().Get("page"), 10, int(unsafe.Sizeof(page)))
		if err == nil {
			page = uint(parsedPage)
		}
	}

	request := &getroles.Request{
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
