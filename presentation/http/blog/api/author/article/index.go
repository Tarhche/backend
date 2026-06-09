package article

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"unsafe"

	"github.com/gofrs/uuid/v5"

	getArticlesByAuthor "github.com/khanzadimahdi/testproject/application/article/getArticlesByAuthor"
	"github.com/khanzadimahdi/testproject/domain"
)

type indexHandler struct {
	useCase *getArticlesByAuthor.UseCase
}

func NewIndexHandler(useCase *getArticlesByAuthor.UseCase) *indexHandler {
	return &indexHandler{
		useCase: useCase,
	}
}

// @Summary		List articles by author
// @Description	return a page of the most recent published articles for the given author identity (UUID or username)
// @Tags		authors
// @Accept		json
// @Produce		json
// @Param		identity	    path		string	true	"Author UUID or username"
// @Param		page		    query		int		false	"Page number"	default(1)
// @Param		language_code	query		string	false	"Language key (e.g. EN, FA)"	default(EN)
// @Success		200			{object}	getArticlesByAuthor.Response
// @Failure		400			{object}	map[string]interface{}
// @Failure		404			{object}	map[string]interface{}
// @Failure		500			{object}	map[string]interface{}
// @Router			/authors/{identity}/articles [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var page uint = 1
	if r.URL.Query().Has("page") {
		parsedPage, err := strconv.ParseUint(r.URL.Query().Get("page"), 10, int(unsafe.Sizeof(page)))
		if err == nil {
			page = uint(parsedPage)
		}
	}

	request := &getArticlesByAuthor.Request{
		Page:         page,
		LanguageCode: r.URL.Query().Get("language_code"),
	}

	identity := r.PathValue("identity")
	if _, err := uuid.FromString(identity); err == nil {
		request.AuthorUUID = identity
	} else {
		request.Username = identity
	}

	response, err := h.useCase.Execute(request)

	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
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
