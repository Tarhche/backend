package hashtag

import (
	"encoding/json"
	"net/http"
	"strconv"
	"unsafe"

	"github.com/julienschmidt/httprouter"
	getArticlesByHashtag "github.com/khanzadimahdi/testproject/application/article/getArticlesByHashtag"
)

type showHandler struct {
	getArticlesByHashtagUseCase *getArticlesByHashtag.UseCase
}

func NewShowHandler(getArticlesByHashtagUseCase *getArticlesByHashtag.UseCase) *showHandler {
	return &showHandler{
		getArticlesByHashtagUseCase: getArticlesByHashtagUseCase,
	}
}

func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var page uint = 1
	if r.URL.Query().Has("page") {
		parsedPage, err := strconv.ParseUint(r.URL.Query().Get("page"), 10, int(unsafe.Sizeof(page)))
		if err == nil {
			page = uint(parsedPage)
		}
	}

	hashtag := httprouter.ParamsFromContext(r.Context()).ByName("hashtag")

	request := &getArticlesByHashtag.Request{
		Page:    page,
		Hashtag: hashtag,
	}

	response, err := h.getArticlesByHashtagUseCase.Execute(request)
	switch true {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
