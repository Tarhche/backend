package presentation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/Tarhche/backend/domain/article"
	"github.com/Tarhche/backend/tests/doubles"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestGetArticles(t *testing.T) {
	t.Run("returns a list of articles", func(t *testing.T) {
		articles := []article.Entity{
			{
				Title: "Lorem Ipsum 1",
			},
			{
				Title: "Lorem Ipsum 2",
			},
			{
				Title: "Lorem Ipsum 3",
			},
		}

		renderer := &doubles.SpyArticleRenderer{}
		server := NewArticleServer(&doubles.StubArticleRepository{Entities: articles}, renderer)

		request, _ := http.NewRequest(http.MethodGet, RoutingPath, nil)
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)

		gotContentType := response.Header().Get("content-type")
		wantContentType := "text/html; charset=UTF-8"

		if gotContentType != wantContentType {
			t.Errorf("got content-type %s want %s", gotContentType, wantContentType)
		}

		if renderer.CallRenderIndexCounter != 1 {
			t.Errorf("got %d wanted %d", renderer.CallRenderIndexCounter, 1)
		}
	})

	t.Run("returns 404 on wrong http method", func(t *testing.T) {
		server := NewArticleServer(&doubles.StubArticleRepository{Entities: []article.Entity{}}, &doubles.SpyArticleRenderer{})

		request, _ := http.NewRequest(http.MethodPatch, RoutingPath, nil)
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)

		got := response.Code
		want := http.StatusNotFound

		if got != want {
			t.Errorf("got HTTP status code %d, want %d", got, want)
		}
	})
}

func TestCreateArticle(t *testing.T) {
	t.Run("creates new article", func(t *testing.T) {
		renderer := &doubles.SpyArticleRenderer{}
		server := NewArticleServer(&doubles.StubArticleRepository{Entities: []article.Entity{}}, renderer)

		anArticle := article.Entity{
			Title:  "title",
			Body:   "body",
			Status: "draft",
		}

		body, _ := json.Marshal(anArticle)
		request, _ := http.NewRequest(http.MethodPost, RoutingPath, bytes.NewReader(body))
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)

		if response.Code != http.StatusCreated {
			t.Errorf("got HTTP status code %d, want %d", response.Code, http.StatusCreated)
		}

		request, _ = http.NewRequest(http.MethodGet, RoutingPath, bytes.NewReader(body))
		response = httptest.NewRecorder()

		server.ServeHTTP(response, request)

		if renderer.CallRenderIndexCounter != 1 {
			t.Errorf("got %d wanted %d", renderer.CallRenderIndexCounter, 1)
		}
	})
}

func TestGetArticle(t *testing.T) {
	anArticle := article.Entity{
		ID:    "id",
		Title: "title",
		Body:  "body",
	}

	renderer := &doubles.SpyArticleRenderer{}
	server := NewArticleServer(&doubles.StubArticleRepository{Entities: []article.Entity{anArticle}}, renderer)

	t.Run("gets an existence article", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/%v", RoutingPath, anArticle.ID), nil)
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)

		gotContentType := response.Header().Get("content-type")
		wantContentType := "text/html; charset=UTF-8"

		if gotContentType != wantContentType {
			t.Errorf("got content-type %s want %s", gotContentType, wantContentType)
		}

		if renderer.CallRenderCounter != 1 {
			t.Errorf("got %d wanted %d", renderer.CallRenderCounter, 1)
		}
	})

	t.Run("gets an non-existance article", func(t *testing.T) {
		id := "non-existance-id"
		request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/articles/%v", id), nil)
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)

		if response.Code != http.StatusNotFound {
			t.Errorf("got HTTP status code %d, want %d", response.Code, http.StatusNotFound)
		}
	})
}

func TestUpdateArticle(t *testing.T) {
	t.Run("updates an article", func(t *testing.T) {
		anArticle := article.Entity{
			ID:    "id",
			Title: "title",
			Body:  "body",
		}

		renderer := &doubles.SpyArticleRenderer{}
		articleRepository := &doubles.StubArticleRepository{Entities: []article.Entity{anArticle}}

		server := NewArticleServer(articleRepository, renderer)

		anArticle.Title = "test title"
		anArticle.Body = "test body"

		body, _ := json.Marshal(anArticle)
		request, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/%s", RoutingPath, anArticle.ID), bytes.NewReader(body))
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Errorf("got HTTP status code %d, want %d", response.Code, http.StatusNoContent)
		}

		got, err := articleRepository.Article(anArticle.ID)
		if err != nil {
			t.Fatal(err)
		}

		if reflect.DeepEqual(got, anArticle) {
			t.Errorf("got %#v, want %#v", got, anArticle)
		}

	})
}

func TestDeleteArticle(t *testing.T) {
	t.Run("deletes an article", func(t *testing.T) {
		anArticle := article.Entity{
			ID:    "id",
			Title: "title",
			Body:  "body",
		}

		renderer := &doubles.SpyArticleRenderer{}
		articleRepository := &doubles.StubArticleRepository{Entities: []article.Entity{anArticle}}

		server := NewArticleServer(articleRepository, renderer)

		request, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/%s", RoutingPath, anArticle.ID), nil)
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)

		if response.Code != http.StatusNoContent {
			t.Errorf("got status %d, wanted %d", response.Code, http.StatusNoContent)
		}

		request, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("%s/%s", RoutingPath, anArticle.ID), nil)
		response = httptest.NewRecorder()

		server.ServeHTTP(response, request)

		if response.Code != http.StatusNotFound {
			t.Errorf("got %d, want %d", response.Code, http.StatusNotFound)
		}
	})
}
