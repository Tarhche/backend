package createComment

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
				Body:       "This is a comment",
				AuthorUUID: "author-uuid-123",
				ParentUUID: "parent-uuid-456",
				ObjectUUID: "article-uuid-789",
				ObjectType: comment.ObjectTypeArticle,
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "valid request without parent",
			request: Request{
				Body:       "This is a comment",
				AuthorUUID: "author-uuid-123",
				ParentUUID: "",
				ObjectUUID: "article-uuid-789",
				ObjectType: comment.ObjectTypeArticle,
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "invalid request with empty body",
			request: Request{
				Body:       "",
				AuthorUUID: "author-uuid-123",
				ParentUUID: "parent-uuid-456",
				ObjectUUID: "article-uuid-789",
				ObjectType: comment.ObjectTypeArticle,
			},
			want: domain.ValidationErrors{
				"body": "required_field",
			},
		},
		{
			name: "invalid request with invalid object type",
			request: Request{
				Body:       "This is a comment",
				AuthorUUID: "author-uuid-123",
				ParentUUID: "parent-uuid-456",
				ObjectUUID: "article-uuid-789",
				ObjectType: "invalid-type",
			},
			want: domain.ValidationErrors{
				"object_type": "invalid_value",
			},
		},
		{
			name: "invalid request with empty object uuid",
			request: Request{
				Body:       "This is a comment",
				AuthorUUID: "author-uuid-123",
				ParentUUID: "parent-uuid-456",
				ObjectUUID: "",
				ObjectType: comment.ObjectTypeArticle,
			},
			want: domain.ValidationErrors{
				"object_uuid": "required_field",
			},
		},
		{
			name: "invalid request with multiple errors",
			request: Request{
				Body:       "",
				AuthorUUID: "author-uuid-123",
				ParentUUID: "parent-uuid-456",
				ObjectUUID: "",
				ObjectType: "invalid-type",
			},
			want: domain.ValidationErrors{
				"body":        "required_field",
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

