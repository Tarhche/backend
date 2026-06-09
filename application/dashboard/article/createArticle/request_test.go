package createarticle

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain"
)

func TestRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request Request
		want    domain.ValidationErrors
	}{
		{
			name: "valid request",
			request: Request{
				Cover:        "cover.jpg",
				Title:        "Test Article",
				Video:        "video.mp4",
				Excerpt:      "This is an excerpt",
				Body:         "This is the body",
				PublishedAt:  time.Now(),
				AuthorUUID:   "author-uuid-123",
				Tags:         []string{"golang", "testing"},
				LanguageCode: "EN",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "valid request with empty optional fields",
			request: Request{
				Title:        "Test Article",
				Excerpt:      "This is an excerpt",
				Body:         "This is the body",
				PublishedAt:  time.Now(),
				AuthorUUID:   "author-uuid-123",
				LanguageCode: "FA",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "invalid request with empty title",
			request: Request{
				Title:        "",
				Excerpt:      "This is an excerpt",
				Body:         "This is the body",
				PublishedAt:  time.Now(),
				AuthorUUID:   "author-uuid-123",
				LanguageCode: "EN",
			},
			want: domain.ValidationErrors{
				"title": "required_field",
			},
		},
		{
			name: "invalid request with empty excerpt",
			request: Request{
				Title:        "Test Article",
				Excerpt:      "",
				Body:         "This is the body",
				PublishedAt:  time.Now(),
				AuthorUUID:   "author-uuid-123",
				LanguageCode: "EN",
			},
			want: domain.ValidationErrors{
				"excerpt": "required_field",
			},
		},
		{
			name: "invalid request with empty body",
			request: Request{
				Title:        "Test Article",
				Excerpt:      "This is an excerpt",
				Body:         "",
				PublishedAt:  time.Now(),
				AuthorUUID:   "author-uuid-123",
				LanguageCode: "EN",
			},
			want: domain.ValidationErrors{
				"body": "required_field",
			},
		},
		{
			name: "invalid request with empty author uuid",
			request: Request{
				Title:        "Test Article",
				Excerpt:      "This is an excerpt",
				Body:         "This is the body",
				PublishedAt:  time.Now(),
				AuthorUUID:   "",
				LanguageCode: "EN",
			},
			want: domain.ValidationErrors{
				"author_uuid": "required_field",
			},
		},
		{
			name: "invalid request with empty language",
			request: Request{
				Title:        "Test Article",
				Excerpt:      "This is an excerpt",
				Body:         "This is the body",
				PublishedAt:  time.Now(),
				AuthorUUID:   "author-uuid-123",
				LanguageCode: "",
			},
			want: domain.ValidationErrors{
				"language_code": "required_field",
			},
		},
		{
			name: "invalid request with multiple errors",
			request: Request{
				Title:        "",
				Excerpt:      "",
				Body:         "",
				PublishedAt:  time.Now(),
				AuthorUUID:   "",
				LanguageCode: "",
			},
			want: domain.ValidationErrors{
				"title":         "required_field",
				"excerpt":       "required_field",
				"body":          "required_field",
				"author_uuid":   "required_field",
				"language_code": "required_field",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.request.Validate()
			assert.Equal(t, tt.want, got)
		})
	}
}
