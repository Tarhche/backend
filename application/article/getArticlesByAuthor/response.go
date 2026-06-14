package getArticlesByAuthor

import (
	"time"

	"github.com/khanzadimahdi/testproject/application/element"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	Author       authorResponse     `json:"author"`
	LanguageCode languageResponse   `json:"language_code"`
	Items        []articleResponse  `json:"items"`
	Elements     []element.Response `json:"elements"`
	Pagination   paginationResponse `json:"pagination"`
}

type articleResponse struct {
	CorrelationUUID    string             `json:"correlation_uuid"`
	Cover              string             `json:"cover"`
	Video              string             `json:"video"`
	Title              string             `json:"title"`
	Excerpt            string             `json:"excerpt"`
	PublishedAt        string             `json:"published_at"`
	AvailableLanguages []languageResponse `json:"available_languages"`
}

type authorResponse struct {
	UUID      string `json:"uuid"`
	Name      string `json:"name"`
	Avatar    string `json:"avatar"`
	Username  string `json:"username"`
	CreatedAt string `json:"created_at"`
}

type languageResponse struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type paginationResponse struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

func NewResponse(author user.User, a []article.Article, articlesPublishedLanguages map[string][]language.Language, requestedLanguage language.Language, elementsResponse []element.Response, totalPages, currentPage uint) *Response {
	items := make([]articleResponse, len(a))

	for i := range a {
		items[i].CorrelationUUID = a[i].CorrelationUUID
		items[i].Cover = a[i].Cover
		items[i].Video = a[i].Video
		items[i].Title = a[i].Title
		items[i].Excerpt = a[i].Excerpt
		items[i].PublishedAt = a[i].PublishedAt.Format(time.RFC3339)

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
		Author: authorResponse{
			UUID:      author.UUID,
			Name:      author.Name,
			Avatar:    author.Avatar,
			Username:  author.Username,
			CreatedAt: author.CreatedAt.Format(time.RFC3339),
		},
		LanguageCode: languageResponse{
			Code: requestedLanguage.Code,
			Name: requestedLanguage.Name,
		},
		Items:    items,
		Elements: elementsResponse,
		Pagination: paginationResponse{
			TotalPages:  totalPages,
			CurrentPage: currentPage,
		},
	}
}
