package getArticlesByHashtag

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	Items        []articleResponse  `json:"items"`
	LanguageCode languageResponse   `json:"language_code"`
	Pagination   paginationResponse `json:"pagination"`
}

type articleResponse struct {
	UUID               string             `json:"uuid"`
	Cover              string             `json:"cover"`
	Video              string             `json:"video"`
	Title              string             `json:"title"`
	Excerpt            string             `json:"excerpt"`
	PublishedAt        string             `json:"published_at"`
	Author             authorResponse     `json:"author"`
	AvailableLanguages []languageResponse `json:"available_languages"`
}

type authorResponse struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name"`
	Avatar   string `json:"avatar"`
	Username string `json:"username"`
}

type languageResponse struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type paginationResponse struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

func NewResponse(a []article.Article, authors []user.User, articlesPublishedLanguages map[string][]language.Language, requestedLanguage language.Language, totalPages, currentPage uint) *Response {
	authorByUUID := make(map[string]user.User, len(authors))
	for i := range authors {
		authorByUUID[authors[i].UUID] = authors[i]
	}

	items := make([]articleResponse, len(a))

	for i := range a {
		items[i].UUID = a[i].UUID
		items[i].Cover = a[i].Cover
		items[i].Video = a[i].Video
		items[i].Title = a[i].Title
		items[i].Excerpt = a[i].Excerpt
		items[i].PublishedAt = a[i].PublishedAt.Format(time.RFC3339)

		if u, ok := authorByUUID[a[i].AuthorUUID]; ok {
			items[i].Author.UUID = u.UUID
			items[i].Author.Name = u.Name
			items[i].Author.Avatar = u.Avatar
			items[i].Author.Username = u.Username
		}

		if al, ok := articlesPublishedLanguages[a[i].UUID]; ok {
			for l := range al {
				items[i].AvailableLanguages = append(items[i].AvailableLanguages, languageResponse{
					Code: al[l].Code,
					Name: al[l].Name,
				})
			}
		}
	}

	return &Response{
		Items: items,
		LanguageCode: languageResponse{
			Code: requestedLanguage.Code,
			Name: requestedLanguage.Name,
		},
		Pagination: paginationResponse{
			TotalPages:  totalPages,
			CurrentPage: currentPage,
		},
	}
}
