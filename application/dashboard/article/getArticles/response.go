package getarticles

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type Response struct {
	Items      []articleResponse `json:"items"`
	Pagination pagination        `json:"pagination"`
}

type articleResponse struct {
	CorrelationUUID string                      `json:"correlation_uuid"`
	CorrolatedItems []corrolatedArticleResponse `json:"corrolated_items"`
}

type corrolatedArticleResponse struct {
	Cover       string `json:"cover"`
	Video       string `json:"video"`
	Title       string `json:"title"`
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
	articles []article.Article,
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

	// group the articles (one per language) under their correlation uuid,
	// preserving the order articles arrive in within each group.
	itemsByCorrelation := make(map[string][]corrolatedArticleResponse, len(correlationUUIDs))
	for i := range articles {
		item := corrolatedArticleResponse{
			Cover:       articles[i].Cover,
			Video:       articles[i].Video,
			Title:       articles[i].Title,
			PublishedAt: articles[i].PublishedAt.Format(time.RFC3339),
			Language: languageResponse{
				Code: articles[i].LanguageCode,
				Name: languageByCode[articles[i].LanguageCode].Name,
			},
		}

		if u, ok := authorByUUID[articles[i].AuthorUUID]; ok {
			item.Author = author{
				UUID:     u.UUID,
				Name:     u.Name,
				Avatar:   u.Avatar,
				Username: u.Username,
			}
		}

		itemsByCorrelation[articles[i].CorrelationUUID] = append(itemsByCorrelation[articles[i].CorrelationUUID], item)
	}

	// emit one entry per correlation uuid, keeping the page order.
	items := make([]articleResponse, 0, len(correlationUUIDs))
	for _, correlationUUID := range correlationUUIDs {
		items = append(items, articleResponse{
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
