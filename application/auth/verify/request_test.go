package verify

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
				Token:      "valid-token",
				Name:       "John Doe",
				Username:   "johndoe",
				Password:   "password123",
				Repassword: "password123",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "invalid request with empty token",
			request: Request{
				Token:      "",
				Name:       "John Doe",
				Username:   "johndoe",
				Password:   "password123",
				Repassword: "password123",
			},
			want: domain.ValidationErrors{
				"token": "required_field",
			},
		},
		{
			name: "invalid request with empty name",
			request: Request{
				Token:      "valid-token",
				Name:       "",
				Username:   "johndoe",
				Password:   "password123",
				Repassword: "password123",
			},
			want: domain.ValidationErrors{
				"name": "required_field",
			},
		},
		{
			name: "invalid request with empty username",
			request: Request{
				Token:      "valid-token",
				Name:       "John Doe",
				Username:   "",
				Password:   "password123",
				Repassword: "password123",
			},
			want: domain.ValidationErrors{
				"username": "required_field",
			},
		},
		{
			name: "invalid request with empty password",
			request: Request{
				Token:      "valid-token",
				Name:       "John Doe",
				Username:   "johndoe",
				Password:   "",
				Repassword: "password123",
			},
			want: domain.ValidationErrors{
				"password":   "required_field",
				"repassword": "repassword",
			},
		},
		{
			name: "invalid request with empty repassword",
			request: Request{
				Token:      "valid-token",
				Name:       "John Doe",
				Username:   "johndoe",
				Password:   "password123",
				Repassword: "",
			},
			want: domain.ValidationErrors{
				"repassword": "repassword",
			},
		},
		{
			name: "invalid request with mismatched passwords",
			request: Request{
				Token:      "valid-token",
				Name:       "John Doe",
				Username:   "johndoe",
				Password:   "password123",
				Repassword: "password456",
			},
			want: domain.ValidationErrors{
				"repassword": "repassword",
			},
		},
		{
			name: "invalid request with different length passwords",
			request: Request{
				Token:      "valid-token",
				Name:       "John Doe",
				Username:   "johndoe",
				Password:   "password123",
				Repassword: "password1234",
			},
			want: domain.ValidationErrors{
				"repassword": "repassword",
			},
		},
		{
			name: "invalid request with multiple errors",
			request: Request{
				Token:      "",
				Name:       "",
				Username:   "",
				Password:   "",
				Repassword: "",
			},
			want: domain.ValidationErrors{
				"token":      "required_field",
				"name":       "required_field",
				"username":   "required_field",
				"password":   "required_field",
				"repassword": "repassword",
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
