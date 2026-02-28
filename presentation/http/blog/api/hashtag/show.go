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

// @Summary		List articles by hashtag
// @Description	return a page of the most recent published articles with the given hashtag
// @Tags			hashtags
// @Accept			json
// @Produce		json
// @Param			hashtag	path		string	true	"Hashtag"
// @Param			page	query		int		false	"Page number"	default(1)
// @Success		200		{object}	getArticlesByHashtag.Response
// @Failure		400		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/hashtags/{hashtag} [get]
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
	case len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
