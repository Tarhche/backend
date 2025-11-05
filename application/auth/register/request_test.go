package register

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
			name: "valid request with valid email",
			request: Request{
				Identity: "user@example.com",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "valid request with valid email containing plus",
			request: Request{
				Identity: "user+tag@example.com",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "valid request with valid email containing hyphen",
			request: Request{
				Identity: "user-name@example.co.uk",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "invalid request with empty identity",
			request: Request{
				Identity: "",
			},
			want: domain.ValidationErrors{
				"identity": "invalid_email",
			},
		},
		{
			name: "invalid request with invalid email format",
			request: Request{
				Identity: "notanemail",
			},
			want: domain.ValidationErrors{
				"identity": "invalid_email",
			},
		},
		{
			name: "invalid request with missing @",
			request: Request{
				Identity: "user.example.com",
			},
			want: domain.ValidationErrors{
				"identity": "invalid_email",
			},
		},
		{
			name: "invalid request with uppercase letters",
			request: Request{
				Identity: "User@Example.com",
			},
			want: domain.ValidationErrors{
				"identity": "invalid_email",
			},
		},
		{
			name: "invalid request with missing domain",
			request: Request{
				Identity: "user@",
			},
			want: domain.ValidationErrors{
				"identity": "invalid_email",
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

