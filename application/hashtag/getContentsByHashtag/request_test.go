package getContentsByHashtag

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
		{
			name: "valid request asking for the articles tab",
			request: Request{
				Hashtag: "golang",
				Type:    TypeArticle,
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "valid request asking for the notes tab",
			request: Request{
				Hashtag: "golang",
				Type:    TypeNote,
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "invalid request with an unknown type",
			request: Request{
				Hashtag: "golang",
				Type:    "video",
			},
			want: domain.ValidationErrors{
				"type": "invalid_value",
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
