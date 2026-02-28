package file

import (
	"encoding/json"
	"net/http"
	"strconv"
	"unsafe"

	"github.com/khanzadimahdi/testproject/application/auth"
	getuserfiles "github.com/khanzadimahdi/testproject/application/dashboard/file/getUserFiles"
)

type indexUserHandler struct {
	useCase *getuserfiles.UseCase
}

func NewIndexUserHandler(useCase *getuserfiles.UseCase) *indexUserHandler {
	return &indexUserHandler{
		useCase: useCase,
	}
}

// @Summary		List own files
// @Description	paginated list of files owned by current user
// @Tags			dashboard files
// @Accept			json
// @Produce		json
// @Param			page	query		int	false	"Page"	default(1)
// @Success		200		{object}	getuserfiles.Response
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/files/user [get]
func (h *indexUserHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID

	var page uint = 1
	if r.URL.Query().Has("page") {
		parsedPage, err := strconv.ParseUint(r.URL.Query().Get("page"), 10, int(unsafe.Sizeof(page)))
		if err == nil {
			page = uint(parsedPage)
		}
	}

	request := &getuserfiles.Request{
		OwnerUUID: userUUID,
		Page:      page,
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
