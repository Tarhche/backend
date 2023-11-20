package article

import (
	"encoding/json"
	"log"
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

	response, err := h.getArticlesUseCase.GetArticles(request)
	switch true {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		log.Println(response)
		json.NewEncoder(rw).Encode(response)
	}
}
