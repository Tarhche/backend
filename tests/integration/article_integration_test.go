package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/Tarhche/backend/domain/article"
	renderer "github.com/Tarhche/backend/infrastructure/renderer/article"
	repository "github.com/Tarhche/backend/infrastructure/repository/article"
	"github.com/Tarhche/backend/presentation"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestCreatingAndRetrievingThem(t *testing.T) {
	articlesRepository := repository.NewInMemoryRepository()
	server := presentation.NewArticleServer(articlesRepository, renderer.NewHTMLArticleRenderer())

	article := article.Entity{
		Title:  "title",
		Body:   "body",
		Status: "draft",
	}

	body, _ := json.Marshal(article)

	request1, _ := http.NewRequest(http.MethodPost, presentation.RoutingPath, bytes.NewReader(body))
	request2, _ := http.NewRequest(http.MethodPost, presentation.RoutingPath, bytes.NewReader(body))
	request3, _ := http.NewRequest(http.MethodPost, presentation.RoutingPath, bytes.NewReader(body))

	server.ServeHTTP(httptest.NewRecorder(), request1)
	server.ServeHTTP(httptest.NewRecorder(), request2)
	server.ServeHTTP(httptest.NewRecorder(), request3)

	request, _ := http.NewRequest(http.MethodGet, presentation.RoutingPath, nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Errorf("got %d, want %d", response.Code, http.StatusOK)
	}

	if response.Body.Len() == 0 {
		t.Errorf("got empty response")
	}

	articles, err := articlesRepository.Articles()
	if err != nil {
		t.Fatal(err)
	}

	// update
	article.ID = articles[0].ID
	article.Status = "published"

	body, _ = json.Marshal(article)

	request, _ = http.NewRequest(http.MethodPut, fmt.Sprintf("%s/%s", presentation.RoutingPath, articles[0].ID), bytes.NewReader(body))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)

	editedArticle := articles[0]

	if !reflect.DeepEqual(editedArticle, article) {
		t.Errorf("got %#v, want %#v", editedArticle, article)
	}

	// delete
	ID := articles[0].ID
	request, _ = http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/%s", presentation.RoutingPath, ID), nil)
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)

	request, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("%s/%s", presentation.RoutingPath, ID), nil)
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Errorf("got status %d, want %d", response.Code, http.StatusNotFound)
	}
}
