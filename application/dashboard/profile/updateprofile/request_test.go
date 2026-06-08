package updateprofile

import (
	"testing"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/stretchr/testify/assert"
)

func TestRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request Request
		want    bool
		wantErr domain.ValidationErrors
	}{
		{
			name: "valid request",
			request: Request{
				UserUUID:     "user-uuid-123",
				Name:         "John Doe",
				Avatar:       "avatar.jpg",
				Email:        "user@example.com",
				Username:     "johndoe",
				LanguageCode: "EN",
			},
			wantErr: domain.ValidationErrors{},
		},
		{
			name: "valid request with empty optional fields",
			request: Request{
				UserUUID:     "user-uuid-123",
				Name:         "John Doe",
				Email:        "user@example.com",
				Username:     "johndoe",
				LanguageCode: "EN",
			},
			wantErr: domain.ValidationErrors{},
		},
		{
			name: "invalid request with empty user uuid",
			request: Request{
				UserUUID:     "",
				Name:         "John Doe",
				Email:        "user@example.com",
				Username:     "johndoe",
				LanguageCode: "EN",
			},
			wantErr: domain.ValidationErrors{
				"uuid": "required_field",
			},
		},
		{
			name: "invalid request with empty name",
			request: Request{
				UserUUID:     "user-uuid-123",
				Name:         "",
				Email:        "user@example.com",
				Username:     "johndoe",
				LanguageCode: "EN",
			},
			wantErr: domain.ValidationErrors{
				"name": "required_field",
			},
		},
		{
			name: "invalid request with empty email",
			request: Request{
				UserUUID:     "user-uuid-123",
				Name:         "John Doe",
				Email:        "",
				Username:     "johndoe",
				LanguageCode: "EN",
			},
			wantErr: domain.ValidationErrors{
				"email": "required_field",
			},
		},
		{
			name: "invalid request with malformed email",
			request: Request{
				UserUUID:     "user-uuid-123",
				Name:         "John Doe",
				Email:        "not-an-email",
				Username:     "johndoe",
				LanguageCode: "EN",
			},
			wantErr: domain.ValidationErrors{
				"email": "invalid_email",
			},
		},
		{
			name: "invalid request with empty username",
			request: Request{
				UserUUID:     "user-uuid-123",
				Name:         "John Doe",
				Email:        "user@example.com",
				Username:     "",
				LanguageCode: "EN",
			},
			wantErr: domain.ValidationErrors{
				"username": "required_field",
			},
		},
		{
			name: "invalid request with empty language",
			request: Request{
				UserUUID:     "user-uuid-123",
				Name:         "John Doe",
				Email:        "user@example.com",
				Username:     "johndoe",
				LanguageCode: "",
			},
			wantErr: domain.ValidationErrors{
				"language_code": "required_field",
			},
		},
		{
			name: "invalid request with multiple errors",
			request: Request{
				UserUUID:     "",
				Name:         "",
				Email:        "",
				Username:     "",
				LanguageCode: "",
			},
			wantErr: domain.ValidationErrors{
				"uuid":          "required_field",
				"name":          "required_field",
				"email":         "required_field",
				"username":      "required_field",
				"language_code": "required_field",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := tt.request.Validate()
			assert.Equal(t, tt.wantErr, gotErr)
		})
	}
}
