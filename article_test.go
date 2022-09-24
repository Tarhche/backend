package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetArticles(t *testing.T) {
	request, _ := http.NewRequest(http.MethodGet, "/articles", nil)
	response := httptest.NewRecorder()

	GetArticles(response, request)

	got := response.Body.String()
	want := "test articles"

	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
