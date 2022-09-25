package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

type StubArticleRepositoy struct{}

func (s *StubArticleRepositoy) Articles() ([]Article, error) {
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

	return articles, nil
}

func TestGetArticles(t *testing.T) {
	articleServer := ArticleServer{
		repository: &StubArticleRepositoy{},
	}

	t.Run("returns a list of articles", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/articles", nil)
		response := httptest.NewRecorder()

		articleServer.ServeHTTP(response, request)

		var got, want []Article

		json.NewDecoder(response.Body).Decode(&got)

		want = []Article{
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

		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	t.Run("creates new article", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodPost, "/articles", nil)
		response := httptest.NewRecorder()

		articleServer.ServeHTTP(response, request)

		got := response.Code
		want := http.StatusCreated

		if got != want {
			t.Errorf("got HTTP status code %d, want %d", got, want)
		}
	})

	t.Run("returns 404 on wrong http method", func(t *testing.T) {
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
