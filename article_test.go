package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
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

		server := NewArticleServer(&StubArticleRepository{articles: articles})

		request, _ := http.NewRequest(http.MethodGet, "/articles", nil)
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)

		var got []Article
		json.NewDecoder(response.Body).Decode(&got)

		if !reflect.DeepEqual(got, articles) {
			t.Errorf("got %#v, want %#v", got, articles)
		}
	})

	t.Run("returns 404 on wrong http method", func(t *testing.T) {
		server := NewArticleServer(&StubArticleRepository{articles: []Article{}})

		request, _ := http.NewRequest(http.MethodPatch, "/articles", nil)
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
		server := NewArticleServer(&StubArticleRepository{articles: []Article{}})

		article := Article{
			Title:  "title",
			Body:   "body",
			Status: "draft",
		}

		body, _ := json.Marshal(article)
		request, _ := http.NewRequest(http.MethodPost, "/articles", bytes.NewReader(body))
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)

		if response.Code != http.StatusCreated {
			t.Errorf("got HTTP status code %d, want %d", response.Code, http.StatusCreated)
		}

		request, _ = http.NewRequest(http.MethodGet, "/articles", bytes.NewReader(body))
		response = httptest.NewRecorder()

		server.ServeHTTP(response, request)

		var got, want []Article

		json.NewDecoder(response.Body).Decode(&got)
		want = append(want, article)

		got[0].ID = want[0].ID // don't check if ID equality
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})
}

func TestGetArticle(t *testing.T) {
	article := Article{
		ID:    "id",
		Title: "title",
		Body:  "body",
	}

	server := NewArticleServer(&StubArticleRepository{articles: []Article{article}})

	t.Run("gets an existance article", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/articles/%v", article.ID), nil)
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)

		var got Article
		json.NewDecoder(response.Body).Decode(&got)

		if response.Code != http.StatusOK {
			t.Errorf("got HTTP status code %d, want %d", response.Code, http.StatusOK)
		}

		if !reflect.DeepEqual(got, article) {
			t.Errorf("got %#v, want %#v", got, article)
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

		server := NewArticleServer(&StubArticleRepository{articles: []Article{article}})

		article.Title = "test title"
		article.Body = "test body"

		body, _ := json.Marshal(article)
		request, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("/articles/%s", article.ID), bytes.NewReader(body))
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Errorf("got HTTP status code %d, want %d", response.Code, http.StatusNoContent)
		}

		request, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("/articles/%s", article.ID), bytes.NewReader(body))
		response = httptest.NewRecorder()

		server.ServeHTTP(response, request)

		got := Article{}
		json.NewDecoder(response.Body).Decode(&got)

		if !reflect.DeepEqual(got, article) {
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

		server := NewArticleServer(&StubArticleRepository{articles: []Article{article}})

		request, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("/articles/%s", article.ID), nil)
		response := httptest.NewRecorder()

		server.ServeHTTP(response, request)

		if response.Code != http.StatusNoContent {
			t.Errorf("got status %d, wanted %d", response.Code, http.StatusNoContent)
		}

		request, _ = http.NewRequest(http.MethodGet, fmt.Sprintf("/articles/%s", article.ID), nil)
		response = httptest.NewRecorder()

		server.ServeHTTP(response, request)

		if response.Code != http.StatusNotFound {
			t.Errorf("got %d, want %d", response.Code, http.StatusNotFound)
		}
	})
}
