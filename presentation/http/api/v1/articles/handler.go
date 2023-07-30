package articles

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gofrs/uuid"
)

type Author struct {
	Name   string
	Avatar string
}

type Article struct {
	UUID        string
	Cover       string
	Title       string
	Body        string
	PublishedAt time.Time
	Author      Author
}

type Pagination struct {
	TotalPages  int
	CurrentPage int
}

type Articles struct {
	Items      []Article
	Pagination Pagination
}

func NewArticlesMux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/articles", getArticles)
	mux.HandleFunc("/articles/", getArticle)

	return mux
}

func getArticle(rw http.ResponseWriter, r *http.Request) {
	u, _ := uuid.NewV7()

	response := Article{
		UUID:        u.String(),
		Cover:       "https://dummyimage.com/640x360/fff/aaa",
		Title:       "this is a test title 3",
		Body:        "</p>this is a paragraph</p>",
		PublishedAt: time.Now(),
		Author: Author{
			Name:   "John Doe",
			Avatar: "https://i.pravatar.cc/150",
		},
	}

	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	json.NewEncoder(rw).Encode(response)
}

func getArticles(rw http.ResponseWriter, r *http.Request) {
	u, _ := uuid.NewV7()

	response := Articles{
		Items: []Article{
			{
				UUID:        u.String(),
				Cover:       "https://dummyimage.com/640x360/fff/aaa",
				Title:       "this is a test title 3",
				Body:        "</p>this is a paragraph</p>",
				PublishedAt: time.Now(),
				Author: Author{
					Name:   "John Doe",
					Avatar: "https://i.pravatar.cc/150",
				},
			},
			{
				UUID:        u.String(),
				Cover:       "https://dummyimage.com/640x360/fff/aaa",
				Title:       "this is a test title 3",
				Body:        "</p>this is a paragraph</p>",
				PublishedAt: time.Now(),
				Author: Author{
					Name:   "John Doe",
					Avatar: "https://i.pravatar.cc/150",
				},
			},
		},
		Pagination: Pagination{
			TotalPages:  2,
			CurrentPage: 1,
		},
	}

	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	json.NewEncoder(rw).Encode(response)
}
