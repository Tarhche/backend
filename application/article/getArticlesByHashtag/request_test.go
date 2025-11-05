package getArticlesByHashtag

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
			name: "valid request with hashtag and page",
			request: Request{
				Hashtag: "golang",
				Page:    1,
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "valid request with empty page",
			request: Request{
				Hashtag: "golang",
				Page:    0,
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "invalid request with empty hashtag",
			request: Request{
				Hashtag: "",
				Page:    1,
			},
			want: domain.ValidationErrors{
				"hashtag": "required_field",
			},
		},
		{
			name: "invalid request with whitespace only hashtag",
			request: Request{
				Hashtag: "   ",
				Page:    1,
			},
			want: domain.ValidationErrors{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.request.Validate()
			assert.Equal(t, tt.want, got)
		})
	}
}

