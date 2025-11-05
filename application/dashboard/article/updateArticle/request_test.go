package updatearticle

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"

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
				UUID:        "article-uuid-123",
				Cover:       "cover.jpg",
				Title:       "Test Article",
				Video:       "video.mp4",
				Excerpt:     "This is an excerpt",
				Body:        "This is the body",
				PublishedAt: time.Now(),
				AuthorUUID:  "author-uuid-123",
				Tags:        []string{"golang", "testing"},
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "valid request with empty optional fields",
			request: Request{
				UUID:        "article-uuid-123",
				Title:       "Test Article",
				Excerpt:     "This is an excerpt",
				Body:        "This is the body",
				PublishedAt: time.Now(),
				AuthorUUID:  "author-uuid-123",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "invalid request with empty title",
			request: Request{
				UUID:        "article-uuid-123",
				Title:       "",
				Excerpt:     "This is an excerpt",
				Body:        "This is the body",
				PublishedAt: time.Now(),
				AuthorUUID:  "author-uuid-123",
			},
			want: domain.ValidationErrors{
				"title": "required_field",
			},
		},
		{
			name: "invalid request with empty excerpt",
			request: Request{
				UUID:        "article-uuid-123",
				Title:       "Test Article",
				Excerpt:     "",
				Body:        "This is the body",
				PublishedAt: time.Now(),
				AuthorUUID:  "author-uuid-123",
			},
			want: domain.ValidationErrors{
				"excerpt": "required_field",
			},
		},
		{
			name: "invalid request with empty body",
			request: Request{
				UUID:        "article-uuid-123",
				Title:       "Test Article",
				Excerpt:     "This is an excerpt",
				Body:        "",
				PublishedAt: time.Now(),
				AuthorUUID:  "author-uuid-123",
			},
			want: domain.ValidationErrors{
				"body": "required_field",
			},
		},
		{
			name: "invalid request with empty author uuid",
			request: Request{
				UUID:        "article-uuid-123",
				Title:       "Test Article",
				Excerpt:     "This is an excerpt",
				Body:        "This is the body",
				PublishedAt: time.Now(),
				AuthorUUID:  "",
			},
			want: domain.ValidationErrors{
				"author": "required_field",
			},
		},
		{
			name: "invalid request with multiple errors",
			request: Request{
				UUID:        "article-uuid-123",
				Title:       "",
				Excerpt:     "",
				Body:        "",
				PublishedAt: time.Now(),
				AuthorUUID:  "",
			},
			want: domain.ValidationErrors{
				"title":   "required_field",
				"excerpt": "required_field",
				"body":    "required_field",
				"author":  "required_field",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
got := tt.request.Validate()
			assert.Equal(t, tt.want, got)


			for key, wantValue := range tt.want {
				if gotValue, ok := got[key]; !ok {
					t.Errorf("Validate() missing error for key %q", key)
				} else if gotValue != wantValue {
					t.Errorf("Validate()[%q] = %q, want %q", key, gotValue, wantValue)
				}
			}

			for key := range got {
				if _, ok := tt.want[key]; !ok {
					t.Errorf("Validate() returned unexpected error for key %q: %q", key, got[key])
				}
			}
		})
	}
}

