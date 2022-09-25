package main

import (
	"log"
	"net/http"
)

func main() {
	server := &ArticleServer{NewInMemoryRepository()}

	log.Fatal(http.ListenAndServe(":8000", server))
}
