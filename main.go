package main

import (
	"log"
	"net/http"
)

func main() {
	server := NewArticleServer(NewInMemoryRepository(), NewHTMLArticleRenderer())

	log.Fatal(http.ListenAndServe(":8000", server))
}
