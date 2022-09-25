package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

type StubArticleRepositoy struct {
	articles []Article
}

func (s *StubArticleRepositoy) Articles() ([]Article, error) {
	return s.articles, nil
}

func (s *StubArticleRepositoy) CreateArticle(article *Article) error {
	s.articles = append(s.articles, *article)

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

		articleServer := ArticleServer{
			repository: &StubArticleRepositoy{
				articles: articles,
			},
		}

		request, _ := http.NewRequest(http.MethodGet, "/articles", nil)
		response := httptest.NewRecorder()

		articleServer.ServeHTTP(response, request)

		got := []Article{}
		json.NewDecoder(response.Body).Decode(&got)

		if !reflect.DeepEqual(got, articles) {
			t.Errorf("got %#v, want %#v", got, articles)
		}
	})

	t.Run("creates new article", func(t *testing.T) {
		articleServer := ArticleServer{
			repository: &StubArticleRepositoy{
				articles: []Article{},
			},
		}

		article := Article{
			Title:  "title",
			Body:   "body",
			Status: "draft",
		}

		body, _ := json.Marshal(article)
		request, _ := http.NewRequest(http.MethodPost, "/articles", bytes.NewReader(body))
		response := httptest.NewRecorder()

		articleServer.ServeHTTP(response, request)

		if response.Code != http.StatusCreated {
			t.Errorf("got HTTP status code %d, want %d", response.Code, http.StatusCreated)
		}

		request, _ = http.NewRequest(http.MethodGet, "/articles", bytes.NewReader(body))
		response = httptest.NewRecorder()

		articleServer.ServeHTTP(response, request)

		var got, want []Article

		json.NewDecoder(response.Body).Decode(&got)
		want = append(want, article)

		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("returns 404 on wrong http method", func(t *testing.T) {
		articleServer := ArticleServer{
			repository: &StubArticleRepositoy{},
		}

		request, _ := http.NewRequest(http.MethodPatch, "/articles", nil)
		response := httptest.NewRecorder()

		articleServer.ServeHTTP(response, request)

		got := response.Code
		want := http.StatusNotFound

		if got != want {
			t.Errorf("got HTTP status code %d, want %d", got, want)
		}
	})
}
