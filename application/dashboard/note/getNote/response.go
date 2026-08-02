package getnote

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/note"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type Response struct {
	Body            string   `json:"body"`
	PublishedAt     string   `json:"published_at"`
	Author          author   `json:"author"`
	Tags            []string `json:"tags"`
	LanguageCode    string   `json:"language_code"`
	CorrelationUUID string   `json:"correlation_uuid"`
}

type author struct {
	UUID     string `json:"uuid"`
	Name     string `json:"name"`
	Avatar   string `json:"avatar"`
	Username string `json:"username"`
}

func NewResponse(n note.Note, u user.User) *Response {
	tags := make([]string, len(n.Tags))
	copy(tags, n.Tags)

	return &Response{
		Body:        n.Body,
		PublishedAt: n.PublishedAt.Format(time.RFC3339),
		Author: author{
			UUID:     u.UUID,
			Name:     u.Name,
			Avatar:   u.Avatar,
			Username: u.Username,
		},
		Tags:            tags,
		LanguageCode:    n.LanguageCode,
		CorrelationUUID: n.CorrelationUUID,
	}
}
