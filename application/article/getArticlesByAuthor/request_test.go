package getArticlesByAuthor

import (
	"testing"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/stretchr/testify/assert"
)

func TestRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request Request
		want    domain.ValidationErrors
	}{
		{
			name: "valid request by username",
			request: Request{
				Username: "johndoe",
				Page:     1,
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "valid request by author uuid",
			request: Request{
				AuthorUUID: "01890d23-7b8e-7e4a-a9bd-1b8a52ad3a01",
				Page:       1,
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "valid request with both username and uuid",
			request: Request{
				AuthorUUID: "01890d23-7b8e-7e4a-a9bd-1b8a52ad3a01",
				Username:   "johndoe",
				Page:       1,
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "valid request with empty page",
			request: Request{
				Username: "johndoe",
				Page:     0,
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "invalid request with neither username nor uuid",
			request: Request{
				Page: 1,
			},
			want: domain.ValidationErrors{
				"author": "required_field",
			},
		},
		{
			name: "invalid request with malformed username",
			request: Request{
				Username: "John Doe!",
				Page:     1,
			},
			want: domain.ValidationErrors{
				"username": "invalid_value",
			},
		},
		{
			name: "invalid request with malformed uuid",
			request: Request{
				AuthorUUID: "not-a-uuid",
				Page:       1,
			},
			want: domain.ValidationErrors{
				"uuid": "invalid_value",
			},
		},
		{
			name: "invalid request with both fields malformed",
			request: Request{
				AuthorUUID: "not-a-uuid",
				Username:   "John Doe!",
				Page:       1,
			},
			want: domain.ValidationErrors{
				"uuid":     "invalid_value",
				"username": "invalid_value",
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
