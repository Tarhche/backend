package main

import (
	"encoding/json"
	"net/http"
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

type ArticleServer struct {
	repository ArticleRepository
}

func (a *ArticleServer) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.articles(rw, r)
	case http.MethodPost:
		a.createArticle(rw, r)
	default:
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
