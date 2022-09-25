package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type Article struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	Status string `json:"status"`
}

type ArticleRepository interface {
	Articles() ([]Article, error)
	CreateArticle(*Article) error
	Article(id string) (*Article, error)
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		articles: []Article{},
	}
}

type InMemoryRepository struct {
	articles []Article
}

func (i *InMemoryRepository) Articles() ([]Article, error) {
	return i.articles, nil
}

func (i *InMemoryRepository) CreateArticle(article *Article) error {
	i.articles = append(i.articles, *article)

	return nil
}

func (i *InMemoryRepository) Article(id string) (*Article, error) {
	for j := range i.articles {
		if i.articles[j].ID == id {
			return &i.articles[j], nil
		}
	}

	return nil, errors.New("article not found")
}

type ArticleServer struct {
	repository ArticleRepository
}

func (a *ArticleServer) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/articles")

	if r.Method == http.MethodPost {
		a.createArticle(rw, r)
	} else if r.Method == http.MethodGet {
		if len(id) == 0 {
			a.articles(rw, r)
		} else {
			a.article(rw, id)
		}
	} else {
		rw.WriteHeader(http.StatusNotFound)
	}
}

func (a *ArticleServer) articles(rw http.ResponseWriter, r *http.Request) {
	articles, _ := a.repository.Articles()
	json.NewEncoder(rw).Encode(articles)
}

func (a *ArticleServer) createArticle(rw http.ResponseWriter, r *http.Request) {
	var article Article

	_ = json.NewDecoder(r.Body).Decode(&article)
	_ = a.repository.CreateArticle(&article)

	rw.WriteHeader(http.StatusCreated)
}

func (a *ArticleServer) article(rw http.ResponseWriter, id string) {
	article, _ := a.repository.Article(id)
	if article == nil {
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	_ = json.NewEncoder(rw).Encode(article)
}
