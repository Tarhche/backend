package main

import (
	"log"
	"net/http"
)

func main() {
	server := &ArticleServer{
		repository: NewInMemoryRepository(),
		router:     http.NewServeMux(),
	}

	log.Fatal(http.ListenAndServe(":8000", server))
}
