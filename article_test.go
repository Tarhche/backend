package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

type StubArticleRepository struct {
	articles []Article
}

func (s *StubArticleRepository) Articles() ([]Article, error) {
	return s.articles, nil
}

func (s *StubArticleRepository) CreateArticle(article *Article) error {
	article.ID = uuid.NewString()
	s.articles = append(s.articles, *article)

	return nil
}

func (s *StubArticleRepository) Article(id string) (*Article, error) {
	for j := range s.articles {
		if s.articles[j].ID == id {
			return &s.articles[j], nil
		}
	}

	return nil, errors.New("article not found")
}

func (s *StubArticleRepository) UpdateArticle(article *Article) error {
	for j := range s.articles {
		if s.articles[j].ID == article.ID {
			s.articles[j] = *article
			return nil
		}
	}

	return errors.New("article not found")
}

func (s *StubArticleRepository) DeleteArticle(ID string) error {
	for j := range s.articles {
		if s.articles[j].ID == ID {
			s.articles[j] = s.articles[len(s.articles)-1]
			s.articles = s.articles[:len(s.articles)-1]

			return nil
		}
	}

	return errors.New("article not found")
}

type SpyArticleRenderer struct {
	CallRenderCounter, CallRenderIndexCounter int
}

func (s *SpyArticleRenderer) Render(w io.Writer, article Article) error {
	s.CallRenderCounter++
	return nil
}

func (s *SpyArticleRenderer) RenderIndex(w io.Writer, articles []Article) error {
	s.CallRenderIndexCounter++
	return nil
}

func TestGetArticles(t *testing.T) {
	t.Run("returns a list of articles", func(t *testing.T) {
		articles := []Article{
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

		renderer := &SpyArticleRenderer{}
		server := NewArticleServer(&StubArticleRepository{articles: articles}, renderer)

		request, _ := http.NewRequest(http.MethodGet, routingPath, nil)
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
		server := NewArticleServer(&StubArticleRepository{articles: []Article{}}, &SpyArticleRenderer{})

		request, _ := http.NewRequest(http.MethodPatch, routingPath, nil)
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
		renderer := &SpyArticleRenderer{}
		server := NewArticleServer(&StubArticleRepository{articles: []Article{}}, renderer)

		article := Article{
			Title:  "title",
			Body:   "body",
			Status: "draft",
		}

		body, _ := json.Marshal(article)
		request, _ := http.NewRequest(http.MethodPost, routingPath, bytes.NewReader(body))
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)

		if response.Code != http.StatusCreated {
			t.Errorf("got HTTP status code %d, want %d", response.Code, http.StatusCreated)
		}

		request, _ = http.NewRequest(http.MethodGet, routingPath, bytes.NewReader(body))
		response = httptest.NewRecorder()

		server.ServeHTTP(response, request)

		if renderer.CallRenderIndexCounter != 1 {
			t.Errorf("got %d wanted %d", renderer.CallRenderIndexCounter, 1)
		}
	})
}

func TestGetArticle(t *testing.T) {
	article := Article{
		ID:    "id",
		Title: "title",
		Body:  "body",
	}

	renderer := &SpyArticleRenderer{}
	server := NewArticleServer(&StubArticleRepository{articles: []Article{article}}, renderer)

	t.Run("gets an existence article", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/%v", routingPath, article.ID), nil)
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
		article := Article{
			ID:    "id",
			Title: "title",
			Body:  "body",
		}

		renderer := &SpyArticleRenderer{}
		articleRepository := &StubArticleRepository{articles: []Article{article}}

		server := NewArticleServer(articleRepository, renderer)

		article.Title = "test title"
		article.Body = "test body"

		body, _ := json.Marshal(article)
		request, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/%s", routingPath, article.ID), bytes.NewReader(body))
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Errorf("got HTTP status code %d, want %d", response.Code, http.StatusNoContent)
		}

		got, err := articleRepository.Article(article.ID)
		if err != nil {
			t.Fatal(err)
		}

		if reflect.DeepEqual(got, article) {
			t.Errorf("got %#v, want %#v", got, article)
		}

	})
}

func TestDeleteArticle(t *testing.T) {
	t.Run("deletes an article", func(t *testing.T) {
		article := Article{
			ID:    "id",
			Title: "title",
			Body:  "body",
		}

		renderer := &SpyArticleRenderer{}
		articleRepository := &StubArticleRepository{articles: []Article{article}}

		server := NewArticleServer(articleRepository, renderer)

		request, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/%s", routingPath, article.ID), nil)
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)

		if response.Code != http.StatusNoContent {
			t.Errorf("got status %d, wanted %d", response.Code, http.StatusNoContent)
		}

		request, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("%s/%s", routingPath, article.ID), nil)
		response = httptest.NewRecorder()

		server.ServeHTTP(response, request)

		if response.Code != http.StatusNotFound {
			t.Errorf("got %d, want %d", response.Code, http.StatusNotFound)
		}
	})
}
