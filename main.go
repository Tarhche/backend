package main

import (
	renderer "github.com/Tarhche/backend/infrastructure/renderer/article"
	"github.com/Tarhche/backend/infrastructure/repository/article"
	"github.com/Tarhche/backend/presentation"
	"log"
	"net/http"
)

func main() {
	server := presentation.NewArticleServer(article.NewInMemoryRepository(), renderer.NewHTMLArticleRenderer())

	log.Fatal(http.ListenAndServe(":8000", server))
}
