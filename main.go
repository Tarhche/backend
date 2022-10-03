package main

import (
	"log"
	"net/http"
)

func main() {
	server := NewArticleServer(NewInMemoryRepository())

	log.Fatal(http.ListenAndServe(":8000", server))
}
