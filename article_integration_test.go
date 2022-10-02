package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestCreatingAndRetrievingThem(t *testing.T) {
	repository := NewInMemoryRepository()
	server := NewArticleServer(repository)

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

	var articles []Article
	json.NewDecoder(response.Body).Decode(&articles)

	if len(articles) != 3 {
		t.Errorf("got %d articles, wanted %d", len(articles), 3)
	}

	// update
	article.ID = articles[0].ID
	article.Status = "published"

	body, _ = json.Marshal(article)

	request, _ = http.NewRequest(http.MethodPut, fmt.Sprintf("/articles/%s", articles[0].ID), bytes.NewReader(body))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)

	request, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("/articles/%s", articles[0].ID), nil)
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)

	var editedArticle Article
	_ = json.NewDecoder(response.Body).Decode(&editedArticle)

	if !reflect.DeepEqual(editedArticle, article) {
		t.Errorf("got %#v, want %#v", editedArticle, article)
	}

	// delete
	request, _ = http.NewRequest(http.MethodDelete, fmt.Sprintf("/articles/%s", articles[0].ID), nil)
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)

	request, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("/articles/%s", articles[0].ID), nil)
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)

	t.Log(response.Body.String(), articles)

	if response.Code != http.StatusNotFound {
		t.Errorf("got status %d, want %d", response.Code, http.StatusNotFound)
	}
}
