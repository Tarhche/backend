package bookmarkExists

import (
	"testing"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/bookmark"
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
				ObjectType: bookmark.ObjectTypeArticle,
				ObjectUUID: "article-uuid-123",
				OwnerUUID:  "owner-uuid-456",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "invalid request with invalid object type",
			request: Request{
				ObjectType: "invalid-type",
				ObjectUUID: "article-uuid-123",
				OwnerUUID:  "owner-uuid-456",
			},
			want: domain.ValidationErrors{
				"object_type": "invalid_value",
			},
		},
		{
			name: "invalid request with empty object uuid",
			request: Request{
				ObjectType: bookmark.ObjectTypeArticle,
				ObjectUUID: "",
				OwnerUUID:  "owner-uuid-456",
			},
			want: domain.ValidationErrors{
				"object_uuid": "required_field",
			},
		},
		{
			name: "invalid request with empty owner uuid",
			request: Request{
				ObjectType: bookmark.ObjectTypeArticle,
				ObjectUUID: "article-uuid-123",
				OwnerUUID:  "",
			},
			want: domain.ValidationErrors{
				"owner_uuid": "required_field",
			},
		},
		{
			name: "invalid request with all errors",
			request: Request{
				ObjectType: "invalid-type",
				ObjectUUID: "",
				OwnerUUID:  "",
			},
			want: domain.ValidationErrors{
				"object_type": "invalid_value",
				"object_uuid": "required_field",
				"owner_uuid":  "required_field",
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
