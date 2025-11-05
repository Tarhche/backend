package login

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
			name: "valid request",
			request: Request{
				Identity: "user@example.com",
				Password: "password123",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "invalid request with empty identity",
			request: Request{
				Identity: "",
				Password: "password123",
			},
			want: domain.ValidationErrors{
				"identity": "required_field",
			},
		},
		{
			name: "invalid request with empty password",
			request: Request{
				Identity: "user@example.com",
				Password: "",
			},
			want: domain.ValidationErrors{
				"password": "required_field",
			},
		},
		{
			name: "invalid request with both empty",
			request: Request{
				Identity: "",
				Password: "",
			},
			want: domain.ValidationErrors{
				"identity": "required_field",
				"password": "required_field",
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

