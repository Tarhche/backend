package home

import (
	"time"

	"github.com/khanzadimahdi/testproject/application/element"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type Response struct {
	All          []articleResponse  `json:"all"`
	Popular      []articleResponse  `json:"popular"`
	Elements     []element.Response `json:"elements"`
	LanguageCode languageResponse   `json:"language_code"`
}

type articleResponse struct {
	UUID        string   `json:"uuid"`
	Cover       string   `json:"cover"`
	Title       string   `json:"title"`
	Author      author   `json:"author"`
	PublishedAt string   `json:"published_at"`
	Excerpt     string   `json:"excerpt"`
	Tags        []string `json:"tags"`
}

type author struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name"`
	Avatar   string `json:"avatar"`
	Username string `json:"username"`
}

type languageResponse struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

func NewResponse(all, popular []article.Article, authors []user.User, requestedLanguage language.Language, elementsResponse []element.Response) *Response {
	authorByUUID := make(map[string]user.User, len(authors))
	for i := range authors {
		authorByUUID[authors[i].UUID] = authors[i]
	}

	return &Response{
		All:      toArticleResponse(all, authorByUUID),
		Popular:  toArticleResponse(popular, authorByUUID),
		Elements: elementsResponse,
		LanguageCode: languageResponse{
			Code: requestedLanguage.Code,
			Name: requestedLanguage.Name,
		},
	}
}

func toArticleResponse(a []article.Article, authors map[string]user.User) []articleResponse {
	items := make([]articleResponse, len(a))

	for i := range a {
		items[i].UUID = a[i].UUID
		items[i].Cover = a[i].Cover
		items[i].Title = a[i].Title
		items[i].Excerpt = a[i].Excerpt
		items[i].Tags = a[i].Tags
		items[i].PublishedAt = a[i].PublishedAt.Format(time.RFC3339)

		if u, ok := authors[a[i].AuthorUUID]; ok {
			items[i].Author.UUID = u.UUID
			items[i].Author.Name = u.Name
			items[i].Author.Avatar = u.Avatar
			items[i].Author.Username = u.Username
		}
	}

	return items
}
