package getContentsByHashtag

import (
	"time"

	"github.com/khanzadimahdi/testproject/application/element"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/note"
	"github.com/khanzadimahdi/testproject/domain/user"
)

// The kinds of content a hashtag page can list.
const (
	TypeArticle = "article"
	TypeNote    = "note"
)

type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	// Type is the kind of content Items holds, and the tab the page should show
	// as selected — the requested one, or the one picked on the caller's behalf.
	Type string `json:"type"`
	// How much of each kind the hashtag has, so the page can label both tabs
	// without asking twice.
	Totals       totalsResponse     `json:"totals"`
	Items        []contentResponse  `json:"items"`
	LanguageCode languageResponse   `json:"language_code"`
	Elements     []element.Response `json:"elements"`
	Pagination   paginationResponse `json:"pagination"`
}

type totalsResponse struct {
	Articles uint `json:"articles"`
	Notes    uint `json:"notes"`
}

// contentResponse carries either an article or a note; Type says which, and the
// fields that don't apply to that type are omitted.
type contentResponse struct {
	Type               string             `json:"type"`
	CorrelationUUID    string             `json:"correlation_uuid"`
	PublishedAt        string             `json:"published_at"`
	Author             authorResponse     `json:"author"`
	AvailableLanguages []languageResponse `json:"available_languages"`

	// articles only
	Cover   string `json:"cover,omitempty"`
	Video   string `json:"video,omitempty"`
	Title   string `json:"title,omitempty"`
	Excerpt string `json:"excerpt,omitempty"`

	// notes only
	Body string   `json:"body,omitempty"`
	Tags []string `json:"tags,omitempty"`
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

// Content is an article or a note, reduced to what the listing needs: the
// fields to render, plus the storage uuid and author to resolve afterwards. It
// lets one code path serve both tabs.
type Content struct {
	UUID            string
	AuthorUUID      string
	CorrelationUUID string

	item contentResponse
}

func NewArticleContent(a article.Article) Content {
	return Content{
		UUID:            a.UUID,
		AuthorUUID:      a.AuthorUUID,
		CorrelationUUID: a.CorrelationUUID,
		item: contentResponse{
			Type:            TypeArticle,
			CorrelationUUID: a.CorrelationUUID,
			PublishedAt:     a.PublishedAt.Format(time.RFC3339),
			Cover:           a.Cover,
			Video:           a.Video,
			Title:           a.Title,
			Excerpt:         a.Excerpt,
		},
	}
}

func NewNoteContent(n note.Note) Content {
	tags := make([]string, len(n.Tags))
	copy(tags, n.Tags)

	return Content{
		UUID:            n.UUID,
		AuthorUUID:      n.AuthorUUID,
		CorrelationUUID: n.CorrelationUUID,
		item: contentResponse{
			Type:            TypeNote,
			CorrelationUUID: n.CorrelationUUID,
			PublishedAt:     n.PublishedAt.Format(time.RFC3339),
			Body:            n.Body,
			Tags:            tags,
		},
	}
}

// IsNote reports whether the content is a note, so callers can look its
// published languages up in the right repository.
func (c Content) IsNote() bool {
	return c.item.Type == TypeNote
}

func NewResponse(
	selectedType string,
	contents []Content,
	authors []user.User,
	publishedLanguages map[string][]language.Language,
	requestedLanguage language.Language,
	elementsResponse []element.Response,
	totalArticles, totalNotes uint,
	totalPages, currentPage uint,
) *Response {
	authorByUUID := make(map[string]user.User, len(authors))
	for i := range authors {
		authorByUUID[authors[i].UUID] = authors[i]
	}

	items := make([]contentResponse, len(contents))

	for i := range contents {
		items[i] = contents[i].item

		if u, ok := authorByUUID[contents[i].AuthorUUID]; ok {
			items[i].Author = authorResponse{
				UUID:     u.UUID,
				Name:     u.Name,
				Avatar:   u.Avatar,
				Username: u.Username,
			}
		}

		if cl, ok := publishedLanguages[contents[i].UUID]; ok {
			for l := range cl {
				items[i].AvailableLanguages = append(items[i].AvailableLanguages, languageResponse{
					Code: cl[l].Code,
					Name: cl[l].Name,
				})
			}
		}
	}

	return &Response{
		Type: selectedType,
		Totals: totalsResponse{
			Articles: totalArticles,
			Notes:    totalNotes,
		},
		Items: items,
		LanguageCode: languageResponse{
			Code: requestedLanguage.Code,
			Name: requestedLanguage.Name,
		},
		Elements: elementsResponse,
		Pagination: paginationResponse{
			TotalPages:  totalPages,
			CurrentPage: currentPage,
		},
	}
}
