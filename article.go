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
	if r.Method == http.MethodGet {
		articles, _ := a.repository.Articles()
		json.NewEncoder(rw).Encode(articles)
	}

	if r.Method == http.MethodPost {
		rw.WriteHeader(http.StatusCreated)
	}

	rw.WriteHeader(http.StatusNotFound)
}
