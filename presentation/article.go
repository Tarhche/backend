package presentation

import (
	"encoding/json"
	"github.com/Tarhche/backend/domain/article"
	"net/http"
	"strings"
)

type ArticleServer struct {
	repository article.Repository
	renderer   article.Renderer
	router     *http.ServeMux
}

const (
	RoutingPath = "/articles"
)

func NewArticleServer(articleRepository article.Repository, renderer article.Renderer) *ArticleServer {
	server := &ArticleServer{
		repository: articleRepository,
		renderer:   renderer,
		router:     http.NewServeMux(),
	}

	server.router.HandleFunc(RoutingPath, func(rw http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			server.articles(rw, r)
		case http.MethodPost:
			server.createArticle(rw, r)
		default:
			http.NotFound(rw, r)
		}
	})

	server.router.HandleFunc(RoutingPath+"/", func(rw http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, RoutingPath+"/")
		if len(id) == 0 {
			http.NotFound(rw, r)
		}

		switch r.Method {
		case http.MethodGet:
			server.article(rw, id)
		case http.MethodPut:
			server.update(rw, r)
		case http.MethodDelete:
			server.delete(rw, id)
		default:
			http.NotFound(rw, r)
		}
	})

	return server
}

func (a *ArticleServer) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	a.router.ServeHTTP(rw, r)
}

func (a *ArticleServer) articles(rw http.ResponseWriter, r *http.Request) {
	articles, _ := a.repository.Articles()

	rw.Header().Set("content-type", "text/html; charset=UTF-8")
	_ = a.renderer.RenderIndex(rw, articles)
}

func (a *ArticleServer) createArticle(rw http.ResponseWriter, r *http.Request) {
	var anArticle article.Entity

	_ = json.NewDecoder(r.Body).Decode(&anArticle)
	_ = a.repository.CreateArticle(&anArticle)

	rw.WriteHeader(http.StatusCreated)
}

func (a *ArticleServer) article(rw http.ResponseWriter, id string) {
	anArticle, err := a.repository.Article(id)
	if err != nil {
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	rw.Header().Set("content-type", "text/html; charset=UTF-8")
	_ = a.renderer.Render(rw, *anArticle)
}

func (a *ArticleServer) update(rw http.ResponseWriter, r *http.Request) {
	anArticle := article.Entity{}

	_ = json.NewDecoder(r.Body).Decode(&anArticle)
	_ = a.repository.UpdateArticle(&anArticle)

	rw.WriteHeader(http.StatusNoContent)
}

func (a *ArticleServer) delete(rw http.ResponseWriter, id string) {
	_ = a.repository.DeleteArticle(id)
	rw.WriteHeader(http.StatusNoContent)
}
