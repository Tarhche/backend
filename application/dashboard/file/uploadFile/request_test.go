package createfile

import (
	"bytes"
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
			name: "valid request",
			request: Request{
				Name:       "test.jpg",
				OwnerUUID:  "owner-uuid-123",
				FileReader: bytes.NewReader([]byte("test content")),
				Size:       100,
				MimeType:   "image/jpeg",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "invalid request with empty name",
			request: Request{
				Name:       "",
				OwnerUUID:  "owner-uuid-123",
				FileReader: bytes.NewReader([]byte("test content")),
				Size:       100,
				MimeType:   "image/jpeg",
			},
			want: domain.ValidationErrors{
				"name": "required_field",
			},
		},
		{
			name: "invalid request with empty owner uuid",
			request: Request{
				Name:       "test.jpg",
				OwnerUUID:  "",
				FileReader: bytes.NewReader([]byte("test content")),
				Size:       100,
				MimeType:   "image/jpeg",
			},
			want: domain.ValidationErrors{
				"owner_uuid": "required_field",
			},
		},
		{
			name: "invalid request with zero size",
			request: Request{
				Name:       "test.jpg",
				OwnerUUID:  "owner-uuid-123",
				FileReader: bytes.NewReader([]byte("test content")),
				Size:       0,
				MimeType:   "image/jpeg",
			},
			want: domain.ValidationErrors{
				"size": "greater_than_zero",
			},
		},
		{
			name: "invalid request with size exceeding limit",
			request: Request{
				Name:       "test.jpg",
				OwnerUUID:  "owner-uuid-123",
				FileReader: bytes.NewReader(make([]byte, 101<<20)),
				Size:       101 << 20,
				MimeType:   "image/jpeg",
			},
			want: domain.ValidationErrors{
				"size": "exceeds_limit",
			},
		},
		{
			name: "valid request with size at limit",
			request: Request{
				Name:       "test.jpg",
				OwnerUUID:  "owner-uuid-123",
				FileReader: bytes.NewReader(make([]byte, 100<<20)),
				Size:       100 << 20,
				MimeType:   "image/jpeg",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "invalid request with multiple errors",
			request: Request{
				Name:       "",
				OwnerUUID:  "",
				FileReader: bytes.NewReader([]byte("test content")),
				Size:       0,
				MimeType:   "image/jpeg",
			},
			want: domain.ValidationErrors{
				"name":       "required_field",
				"owner_uuid": "required_field",
				"size":       "greater_than_zero",
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

func TestRequest_StoredName(t *testing.T) {
	t.Run("generates stored name with extension", func(t *testing.T) {
		req := Request{
			Name: "test.jpg",
		}

		storedName, err := req.StoredName()
		assert.NoError(t, err)
		assert.NotEmpty(t, storedName)
		assert.Contains(t, storedName, ".jpg")
	})

	t.Run("generates stored name without extension for file without extension", func(t *testing.T) {
		req := Request{
			Name: "test",
		}

		storedName, err := req.StoredName()
		assert.NoError(t, err)
		assert.NotEmpty(t, storedName)
	})
}
