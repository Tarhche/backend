package article

import (
	"encoding/json"
	"net/http"
	"strconv"
	"unsafe"

	getarticles "github.com/khanzadimahdi/testproject/application/article/getArticles"
)

type indexHandler struct {
	useCase *getarticles.UseCase
}

func NewIndexHandler(useCase *getarticles.UseCase) *indexHandler {
	return &indexHandler{
		useCase: useCase,
	}
}

// @Summary		List published articles
// @Description	return a page of the most recent published articles
// @Tags			articles
// @Accept			json
// @Produce		json
// @Param			page	query		int	false	"Page number"	default(1)
// @Success		200		{object}	getarticles.Response
// @Failure		500		{object}	map[string]interface{}
// @Router			/articles [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var page uint = 1
	if r.URL.Query().Has("page") {
		parsedPage, err := strconv.ParseUint(r.URL.Query().Get("page"), 10, int(unsafe.Sizeof(page)))
		if err == nil {
			page = uint(parsedPage)
		}
	}

	request := &getarticles.Request{
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
