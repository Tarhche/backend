package createuser

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
				Email:        "user@example.com",
				Name:         "John Doe",
				Avatar:       "avatar.jpg",
				Username:     "johndoe",
				Password:     "password123",
				LanguageCode: "en",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "valid request with empty optional fields",
			request: Request{
				Email:        "user@example.com",
				Name:         "John Doe",
				Username:     "johndoe",
				Password:     "password123",
				LanguageCode: "en",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "invalid request with empty email",
			request: Request{
				Email:        "",
				Name:         "John Doe",
				Username:     "johndoe",
				Password:     "password123",
				LanguageCode: "en",
			},
			want: domain.ValidationErrors{
				"email": "required_field",
			},
		},
		{
			name: "invalid request with malformed email",
			request: Request{
				Email:        "not-an-email",
				Name:         "John Doe",
				Username:     "johndoe",
				Password:     "password123",
				LanguageCode: "en",
			},
			want: domain.ValidationErrors{
				"email": "invalid_email",
			},
		},
		{
			name: "invalid request with empty name",
			request: Request{
				Email:        "user@example.com",
				Name:         "",
				Username:     "johndoe",
				Password:     "password123",
				LanguageCode: "en",
			},
			want: domain.ValidationErrors{
				"name": "required_field",
			},
		},
		{
			name: "invalid request with empty username",
			request: Request{
				Email:        "user@example.com",
				Name:         "John Doe",
				Username:     "",
				Password:     "password123",
				LanguageCode: "en",
			},
			want: domain.ValidationErrors{
				"username": "required_field",
			},
		},
		{
			name: "invalid request with malformed username",
			request: Request{
				Email:        "user@example.com",
				Name:         "John Doe",
				Username:     "John Doe!",
				Password:     "password123",
				LanguageCode: "en",
			},
			want: domain.ValidationErrors{
				"username": "invalid_value",
			},
		},
		{
			name: "invalid request with username having no alphanumerics",
			request: Request{
				Email:        "user@example.com",
				Name:         "John Doe",
				Username:     "...",
				Password:     "password123",
				LanguageCode: "en",
			},
			want: domain.ValidationErrors{
				"username": "invalid_value",
			},
		},
		{
			name: "invalid request with empty password",
			request: Request{
				Email:        "user@example.com",
				Name:         "John Doe",
				Username:     "johndoe",
				Password:     "",
				LanguageCode: "en",
			},
			want: domain.ValidationErrors{
				"password": "required_field",
			},
		},
		{
			name: "invalid request with empty language code",
			request: Request{
				Email:    "user@example.com",
				Name:     "John Doe",
				Username: "johndoe",
				Password: "password123",
			},
			want: domain.ValidationErrors{
				"language_code": "required_field",
			},
		},
		{
			name: "invalid request with multiple errors",
			request: Request{
				Email:        "",
				Name:         "",
				Username:     "",
				Password:     "",
				LanguageCode: "",
			},
			want: domain.ValidationErrors{
				"email":         "required_field",
				"name":          "required_field",
				"username":      "required_field",
				"password":      "required_field",
				"language_code": "required_field",
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
