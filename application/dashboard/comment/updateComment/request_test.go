package updateComment

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/comment"
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
				UUID:       "comment-uuid-123",
				Body:       "Updated comment",
				AuthorUUID: "author-uuid-123",
				ParentUUID: "parent-uuid-456",
				ObjectUUID: "article-uuid-789",
				ObjectType: comment.ObjectTypeArticle,
				ApprovedAt: time.Now(),
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "invalid request with empty uuid",
			request: Request{
				UUID:       "",
				Body:       "Updated comment",
				AuthorUUID: "author-uuid-123",
				ParentUUID: "parent-uuid-456",
				ObjectUUID: "article-uuid-789",
				ObjectType: comment.ObjectTypeArticle,
				ApprovedAt: time.Now(),
			},
			want: domain.ValidationErrors{
				"uuid": "required_field",
			},
		},
		{
			name: "invalid request with empty body",
			request: Request{
				UUID:       "comment-uuid-123",
				Body:       "",
				AuthorUUID: "author-uuid-123",
				ParentUUID: "parent-uuid-456",
				ObjectUUID: "article-uuid-789",
				ObjectType: comment.ObjectTypeArticle,
				ApprovedAt: time.Now(),
			},
			want: domain.ValidationErrors{
				"body": "required_field",
			},
		},
		{
			name: "invalid request with invalid object type",
			request: Request{
				UUID:       "comment-uuid-123",
				Body:       "Updated comment",
				AuthorUUID: "author-uuid-123",
				ParentUUID: "parent-uuid-456",
				ObjectUUID: "article-uuid-789",
				ObjectType: "invalid-type",
				ApprovedAt: time.Now(),
			},
			want: domain.ValidationErrors{
				"object_type": "invalid_value",
			},
		},
		{
			name: "invalid request with empty object uuid",
			request: Request{
				UUID:       "comment-uuid-123",
				Body:       "Updated comment",
				AuthorUUID: "author-uuid-123",
				ParentUUID: "parent-uuid-456",
				ObjectUUID: "",
				ObjectType: comment.ObjectTypeArticle,
				ApprovedAt: time.Now(),
			},
			want: domain.ValidationErrors{
				"object_uuid": "required_field",
			},
		},
		{
			name: "invalid request with multiple errors",
			request: Request{
				UUID:       "",
				Body:       "",
				AuthorUUID: "author-uuid-123",
				ParentUUID: "parent-uuid-456",
				ObjectUUID: "",
				ObjectType: "invalid-type",
				ApprovedAt: time.Now(),
			},
			want: domain.ValidationErrors{
				"uuid":        "required_field",
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

