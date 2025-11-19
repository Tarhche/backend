package createrole

import (
	"github.com/stretchr/testify/assert"
	"testing"

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
				Name:        "Admin",
				Description: "Administrator role",
				Permissions: []string{"read", "write"},
				UserUUIDs:   []string{"user-uuid-1", "user-uuid-2"},
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "valid request with empty optional fields",
			request: Request{
				Name:        "Admin",
				Description: "Administrator role",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "invalid request with empty name",
			request: Request{
				Name:        "",
				Description: "Administrator role",
			},
			want: domain.ValidationErrors{
				"name": "required_field",
			},
		},
		{
			name: "invalid request with empty description",
			request: Request{
				Name:        "Admin",
				Description: "",
			},
			want: domain.ValidationErrors{
				"description": "required_field",
			},
		},
		{
			name: "invalid request with multiple errors",
			request: Request{
				Name:        "",
				Description: "",
			},
			want: domain.ValidationErrors{
				"name":        "required_field",
				"description": "required_field",
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
