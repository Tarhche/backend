package articles

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	getarticle "github.com/khanzadimahdi/testproject.git/application/article/getArticle"
	getarticles "github.com/khanzadimahdi/testproject.git/application/article/getArticles"
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

	return mux
}

type handler struct {
	getArticleUseCase  *getarticle.UseCase
	getArticlesUseCase *getarticles.UseCase
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

	request := &getarticles.Request{
		Page: 1,
	}

	response, _ := h.getArticlesUseCase.GetArticles(request)

	rw.WriteHeader(http.StatusOK)
	json.NewEncoder(rw).Encode(response)
}
