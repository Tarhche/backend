package getarticle

import (
	"time"

	"github.com/khanzadimahdi/testproject/application/element"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type Response struct {
	ValidationErrors map[string]string `json:"validation_errors,omitempty"`

	CorrelationUUID    string             `json:"correlation_uuid"`
	Cover              string             `json:"cover"`
	Video              string             `json:"video"`
	Title              string             `json:"title"`
	Excerpt            string             `json:"excerpt"`
	Body               string             `json:"body"`
	PublishedAt        string             `json:"published_at"`
	Author             authorResponse     `json:"author"`
	Tags               []string           `json:"tags"`
	ViewCount          uint               `json:"view_count"`
	LanguageCode       languageResponse   `json:"language_code"`
	AvailableLanguages []languageResponse `json:"available_languages"`
	Elements           []element.Response `json:"elements"`
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

func NewResponse(a article.Article, language language.Language, author user.User, availableLanguages []language.Language, elementsResponse []element.Response) *Response {
	tags := make([]string, len(a.Tags))
	copy(tags, a.Tags)

	languages := make([]languageResponse, len(availableLanguages))
	for i, l := range availableLanguages {
		languages[i] = languageResponse{
			Code: l.Code,
			Name: l.Name,
		}
	}

	return &Response{
		CorrelationUUID: a.CorrelationUUID,
		Cover:           a.Cover,
		Video:           a.Video,
		Title:           a.Title,
		Excerpt:         a.Excerpt,
		Body:            a.Body,
		PublishedAt:     a.PublishedAt.Format(time.RFC3339),
		Author: authorResponse{
			UUID:     author.UUID,
			Name:     author.Name,
			Avatar:   author.Avatar,
			Username: author.Username,
		},
		Tags:      tags,
		ViewCount: a.ViewCount,
		LanguageCode: languageResponse{
			Code: language.Code,
			Name: language.Name,
		},
		AvailableLanguages: languages,
		Elements:           elementsResponse,
	}
}
