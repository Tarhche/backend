package articles

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unsafe"

	createarticle "github.com/khanzadimahdi/testproject.git/application/article/createArticle"
	deletearticle "github.com/khanzadimahdi/testproject.git/application/article/deleteArticle"
	getarticle "github.com/khanzadimahdi/testproject.git/application/article/getArticle"
	getarticles "github.com/khanzadimahdi/testproject.git/application/article/getArticles"
	updatearticle "github.com/khanzadimahdi/testproject.git/application/article/updateArticle"
	"github.com/khanzadimahdi/testproject.git/domain"
)

func NewArticlesMux(
	getArticle *getarticle.UseCase,
	getArticles *getarticles.UseCase,
) *http.ServeMux {
	h := &handler{
		getArticleUseCase:  getArticle,
		getArticlesUseCase: getArticles,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/articles", h.articles)
	mux.HandleFunc("/articles/", h.article)
	mux.HandleFunc("/articles/create/", h.update)
	mux.HandleFunc("/articles/update/", h.update)
	mux.HandleFunc("/articles/delete/", h.delete)

	return mux
}

type handler struct {
	getArticleUseCase     *getarticle.UseCase
	getArticlesUseCase    *getarticles.UseCase
	createArticlesUseCase *createarticle.UseCase
	updateArticleUseCase  *updatearticle.UseCase
	deleteArticleUseCase  *deletearticle.UseCase
}

func (h *handler) article(rw http.ResponseWriter, r *http.Request) {
	url := strings.TrimPrefix(r.URL.EscapedPath(), "/articles/")
	url = strings.TrimSuffix(url, "/")
	segments := strings.Split(url, "/")

	var UUID string
	if len(segments) > 0 {
		UUID = segments[0]
	}

	response, err := h.getArticleUseCase.GetArticle(UUID)

	switch true {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}

func (h *handler) articles(rw http.ResponseWriter, r *http.Request) {
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

func (h *handler) create(rw http.ResponseWriter, r *http.Request) {
	var request createarticle.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	validationErrors, err := h.createArticlesUseCase.CreateArticle(request)

	switch true {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case len(validationErrors.ValidationErrors) > 0:
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(validationErrors)
	default:
		rw.WriteHeader(http.StatusCreated)
	}
}

func (h *handler) update(rw http.ResponseWriter, r *http.Request) {
	var request updatearticle.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	validationErrors, err := h.updateArticleUseCase.UpdateArticle(request)
	switch true {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case len(validationErrors.ValidationErrors) > 0:
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(validationErrors)
	default:
		rw.WriteHeader(http.StatusOK)
	}
}

func (h *handler) delete(rw http.ResponseWriter, r *http.Request) {
	url := strings.TrimPrefix(r.URL.EscapedPath(), "/articles/")
	url = strings.TrimSuffix(url, "/")
	segments := strings.Split(url, "/")

	var UUID string
	if len(segments) > 0 {
		UUID = segments[0]
	}

	request := deletearticle.Request{
		ArticleUUID: UUID,
	}

	err := h.deleteArticleUseCase.DeleteArticle(request)
	switch true {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	default:
		rw.WriteHeader(http.StatusOK)
	}

	rw.WriteHeader(http.StatusCreated)
}
