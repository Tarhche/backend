package getnotes

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/note"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type Response struct {
	Items      []noteResponse `json:"items"`
	Pagination pagination     `json:"pagination"`
}

type noteResponse struct {
	CorrelationUUID string                   `json:"correlation_uuid"`
	CorrolatedItems []corrolatedNoteResponse `json:"corrolated_items"`
}

type corrolatedNoteResponse struct {
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`

	Author   author           `json:"author"`
	Language languageResponse `json:"language"`
}

type languageResponse struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type author struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name"`
	Avatar   string `json:"avatar"`
	Username string `json:"username"`
}

type pagination struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

func NewResponse(
	correlationUUIDs []string,
	notes []note.Note,
	authors []user.User,
	languages []language.Language,
	totalPages, currentPage uint,
) *Response {
	authorByUUID := make(map[string]user.User, len(authors))
	for i := range authors {
		authorByUUID[authors[i].UUID] = authors[i]
	}

	languageByCode := make(map[string]language.Language, len(languages))
	for i := range languages {
		languageByCode[languages[i].Code] = languages[i]
	}

	// group the notes (one per language) under their correlation uuid,
	// preserving the order notes arrive in within each group.
	itemsByCorrelation := make(map[string][]corrolatedNoteResponse, len(correlationUUIDs))
	for i := range notes {
		item := corrolatedNoteResponse{
			Body:        notes[i].Body,
			PublishedAt: notes[i].PublishedAt.Format(time.RFC3339),
			Language: languageResponse{
				Code: notes[i].LanguageCode,
				Name: languageByCode[notes[i].LanguageCode].Name,
			},
		}

		if u, ok := authorByUUID[notes[i].AuthorUUID]; ok {
			item.Author = author{
				UUID:     u.UUID,
				Name:     u.Name,
				Avatar:   u.Avatar,
				Username: u.Username,
			}
		}

		itemsByCorrelation[notes[i].CorrelationUUID] = append(itemsByCorrelation[notes[i].CorrelationUUID], item)
	}

	// emit one entry per correlation uuid, keeping the page order.
	items := make([]noteResponse, 0, len(correlationUUIDs))
	for _, correlationUUID := range correlationUUIDs {
		items = append(items, noteResponse{
			CorrelationUUID: correlationUUID,
			CorrolatedItems: itemsByCorrelation[correlationUUID],
		})
	}

	return &Response{
		Items: items,
		Pagination: pagination{
			TotalPages:  totalPages,
			CurrentPage: currentPage,
		},
	}
}
