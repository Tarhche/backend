package article

import (
	"encoding/json"
	"net/http"
	"strconv"
	"unsafe"

	getarticles "github.com/khanzadimahdi/testproject.git/application/article/getArticles"
)

type indexHandler struct {
	getArticlesUseCase *getarticles.UseCase
}

func NewIndexHandler(getArticlesUseCase *getarticles.UseCase) *indexHandler {
	return &indexHandler{
		getArticlesUseCase: getArticlesUseCase,
	}
}

func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Add("Content-Type", "application/json")

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

	response, _ := h.getArticlesUseCase.GetArticles(request)

	rw.WriteHeader(http.StatusOK)
	json.NewEncoder(rw).Encode(response)
}
