package main

import (
	"flag"
	renderer "github.com/Tarhche/backend/infrastructure/renderer/article"
	"github.com/Tarhche/backend/infrastructure/repository/article"
	"github.com/Tarhche/backend/presentation"
	"log"
	"net/http"
)

func main() {
	var (
		mongodbURI, mongodbDatabase string
	)

	flag.StringVar(&mongodbURI, "mongodb-uri", "", "connection uri")
	flag.StringVar(&mongodbDatabase, "mongodb-database", "", "mongodb database name")

	flag.Parse()

	server := presentation.NewArticleServer(
		article.NewMongoDBRepository(mongodbURI, mongodbDatabase),
		renderer.NewHTMLArticleRenderer(),
	)

	log.Fatal(http.ListenAndServe(":8000", server))
}
