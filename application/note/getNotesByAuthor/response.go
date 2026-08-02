package getNotesByAuthor

import (
	"time"

	"github.com/khanzadimahdi/testproject/application/element"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/note"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	Author authorResponse `json:"author"`
	// How much of each kind the author has published, so the page can label both
	// content tabs without asking twice.
	Totals       totalsResponse     `json:"totals"`
	LanguageCode languageResponse   `json:"language_code"`
	Items        []noteResponse     `json:"items"`
	Elements     []element.Response `json:"elements"`
	Pagination   paginationResponse `json:"pagination"`
}

type noteResponse struct {
	CorrelationUUID    string             `json:"correlation_uuid"`
	Body               string             `json:"body"`
	PublishedAt        string             `json:"published_at"`
	Tags               []string           `json:"tags"`
	AvailableLanguages []languageResponse `json:"available_languages"`
}

type totalsResponse struct {
	Articles uint `json:"articles"`
	Notes    uint `json:"notes"`
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

func NewResponse(author user.User, n []note.Note, notesPublishedLanguages map[string][]language.Language, requestedLanguage language.Language, elementsResponse []element.Response, totalArticles, totalNotes uint, totalPages, currentPage uint) *Response {
	items := make([]noteResponse, len(n))

	for i := range n {
		items[i].CorrelationUUID = n[i].CorrelationUUID
		items[i].Body = n[i].Body
		items[i].PublishedAt = n[i].PublishedAt.Format(time.RFC3339)

		items[i].Tags = make([]string, len(n[i].Tags))
		copy(items[i].Tags, n[i].Tags)

		if nl, ok := notesPublishedLanguages[n[i].UUID]; ok {
			for l := range nl {
				items[i].AvailableLanguages = append(items[i].AvailableLanguages, languageResponse{
					Code: nl[l].Code,
					Name: nl[l].Name,
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
		Totals: totalsResponse{
			Articles: totalArticles,
			Notes:    totalNotes,
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
