package main

import (
	"encoding/json"
	"net/http"
)

type Article struct {
	Title string `json:"title"`
}

type ArticleRepository interface {
	Articles() ([]Article, error)
}

type InMemoryRepository struct {
}

func (i InMemoryRepository) Articles() ([]Article, error) {
	articles := []Article{
		{
			Title: "Lorem Ipsum 1",
		},
		{
			Title: "Lorem Ipsum 2",
		},
		{
			Title: "Lorem Ipsum 3",
		},
	}

	return articles, nil
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
	rw.WriteHeader(http.StatusCreated)
}
