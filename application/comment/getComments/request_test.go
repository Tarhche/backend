package getComments

import (
	"testing"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/comment"
	"github.com/stretchr/testify/assert"
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
				Page:       1,
				ObjectUUID: "article-uuid-123",
				ObjectType: comment.ObjectTypeArticle,
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "valid request with page zero",
			request: Request{
				Page:       0,
				ObjectUUID: "article-uuid-123",
				ObjectType: comment.ObjectTypeArticle,
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "invalid request with invalid object type",
			request: Request{
				Page:       1,
				ObjectUUID: "article-uuid-123",
				ObjectType: "invalid-type",
			},
			want: domain.ValidationErrors{
				"object_type": "invalid_value",
			},
		},
		{
			name: "invalid request with empty object uuid",
			request: Request{
				Page:       1,
				ObjectUUID: "",
				ObjectType: comment.ObjectTypeArticle,
			},
			want: domain.ValidationErrors{
				"object_uuid": "required_field",
			},
		},
		{
			name: "invalid request with both errors",
			request: Request{
				Page:       1,
				ObjectUUID: "",
				ObjectType: "invalid-type",
			},
			want: domain.ValidationErrors{
				"object_type": "invalid_value",
				"object_uuid": "required_field",
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
