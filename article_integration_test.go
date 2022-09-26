package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreatingAndRetrievingThem(t *testing.T) {
	repository := NewInMemoryRepository()
	server := ArticleServer{repository: repository}
	article := Article{
		Title:  "title",
		Body:   "body",
		Status: "draft",
	}

	body, _ := json.Marshal(article)

	request1, _ := http.NewRequest(http.MethodPost, "/articles", bytes.NewReader(body))
	request2, _ := http.NewRequest(http.MethodPost, "/articles", bytes.NewReader(body))
	request3, _ := http.NewRequest(http.MethodPost, "/articles", bytes.NewReader(body))

	server.ServeHTTP(httptest.NewRecorder(), request1)
	server.ServeHTTP(httptest.NewRecorder(), request2)
	server.ServeHTTP(httptest.NewRecorder(), request3)

	request, _ := http.NewRequest(http.MethodGet, "/articles", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Errorf("got %d, want %d", response.Code, http.StatusOK)
	}

	articles := []Article{}
	json.NewDecoder(response.Body).Decode(&articles)

	if len(articles) != 3 {
		t.Errorf("got %d articles, wanted %d", len(articles), 3)
	}

}
