package articles

import (
	"encoding/json"
	"net/http"

	getarticle "github.com/khanzadimahdi/testproject.git/application/article/getArticle"
	getarticles "github.com/khanzadimahdi/testproject.git/application/article/getArticles"
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
	mux.HandleFunc("/article/", h.article)

	return mux
}

type handler struct {
	getArticleUseCase  *getarticle.UseCase
	getArticlesUseCase *getarticles.UseCase
}

func (h *handler) article(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Add("Content-Type", "application/json")

	response, _ := h.getArticleUseCase.GetArticle("")

	rw.WriteHeader(http.StatusOK)
	json.NewEncoder(rw).Encode(response)
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
