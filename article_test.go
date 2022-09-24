package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestGetArticles(t *testing.T) {
	request, _ := http.NewRequest(http.MethodGet, "/articles", nil)
	response := httptest.NewRecorder()

	GetArticles(response, request)

	var got, want []Article

	json.NewDecoder(response.Body).Decode(&got)
	want = []Article{
		{
			Title: "Lorem Ipsum 1",
		},
		{
			Title: "Lorem Ipsum 1",
		},
		{
			Title: "Lorem Ipsum 1",
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}
