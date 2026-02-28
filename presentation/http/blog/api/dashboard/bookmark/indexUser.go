package bookmark

import (
	"encoding/json"
	"net/http"
	"strconv"
	"unsafe"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/bookmark/getUserBookmarks"
)

type indexUserHandler struct {
	useCase *getUserBookmarks.UseCase
}

func NewIndexUserBookmarksHandler(useCase *getUserBookmarks.UseCase) *indexUserHandler {
	return &indexUserHandler{
		useCase: useCase,
	}
}

// @Summary		List my bookmarks
// @Description	paginated list of bookmarks for authenticated user
// @Tags			dashboard bookmarks
// @Accept			json
// @Produce		json
// @Param			page	query		int	false	"Page"	default(1)
// @Success		200		{object}	getUserBookmarks.Response
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/my/bookmarks [get]
func (h *indexUserHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID

	var page uint = 1
	if r.URL.Query().Has("page") {
		parsedPage, err := strconv.ParseUint(r.URL.Query().Get("page"), 10, int(unsafe.Sizeof(page)))
		if err == nil {
			page = uint(parsedPage)
		}
	}

	request := &getUserBookmarks.Request{
		Page:      page,
		OwnerUUID: userUUID,
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
