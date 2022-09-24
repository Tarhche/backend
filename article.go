package main

import (
	"fmt"
	"net/http"
)

func GetArticles(rw http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(rw, "test articles")
}
