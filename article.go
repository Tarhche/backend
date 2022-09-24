package main

import (
	"encoding/json"
	"net/http"
)

type Article struct {
	Title string `json:"title"`
}

func GetArticles(rw http.ResponseWriter, r *http.Request) {
	articles := []Article{
		{
			Title: "Lorem Ipsum 1",
		},
		{
			Title: "Lorem Ipsum 1",
		},
		{
			Title: "Lorem Ipsum 1",
		},
	}

	json.NewEncoder(rw).Encode(articles)
}
