package comment

import (
	"encoding/json"
	"net/http"
	"strconv"
	"unsafe"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/comment/getUserComments"
)

type indexUserHandler struct {
	useCase *getUserComments.UseCase
}

func NewIndexUserCommentsHandler(useCase *getUserComments.UseCase) *indexUserHandler {
	return &indexUserHandler{
		useCase: useCase,
	}
}

// @Summary		List my comments
// @Description	paginated list of comments authored by current user
// @Tags			dashboard comments
// @Accept			json
// @Produce		json
// @Param			page	query		int	false	"Page"	default(1)
// @Success		200		{object}	getUserComments.Response
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/comments/user [get]
func (h *indexUserHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID

	var page uint = 1
	if r.URL.Query().Has("page") {
		parsedPage, err := strconv.ParseUint(r.URL.Query().Get("page"), 10, int(unsafe.Sizeof(page)))
		if err == nil {
			page = uint(parsedPage)
		}
	}

	request := &getUserComments.Request{
		Page:     page,
		UserUUID: userUUID,
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
