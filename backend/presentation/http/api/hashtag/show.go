package hashtag

import (
	"encoding/json"
	"net/http"
	"strconv"
	"unsafe"

	getArticlesByHashtag "github.com/khanzadimahdi/testproject/application/article/getArticlesByHashtag"
)

type showHandler struct {
	useCase *getArticlesByHashtag.UseCase
}

func NewShowHandler(useCase *getArticlesByHashtag.UseCase) *showHandler {
	return &showHandler{
		useCase: useCase,
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

	hashtag := r.PathValue("hashtag")

	request := &getArticlesByHashtag.Request{
		Page:    page,
		Hashtag: hashtag,
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
