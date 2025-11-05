package updateprofile

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request Request
		want    bool
		wantErr validationErrors
	}{
		{
			name: "valid request",
			request: Request{
				UserUUID: "user-uuid-123",
				Name:     "John Doe",
				Avatar:   "avatar.jpg",
				Email:    "user@example.com",
				Username: "johndoe",
			},
			want:    true,
			wantErr: validationErrors{},
		},
		{
			name: "valid request with empty optional fields",
			request: Request{
				UserUUID: "user-uuid-123",
				Name:     "John Doe",
				Email:    "user@example.com",
				Username: "johndoe",
			},
			want:    true,
			wantErr: validationErrors{},
		},
		{
			name: "invalid request with empty user uuid",
			request: Request{
				UserUUID: "",
				Name:     "John Doe",
				Email:    "user@example.com",
				Username: "johndoe",
			},
			want: false,
			wantErr: validationErrors{
				"uuid": "required_field",
			},
		},
		{
			name: "invalid request with empty name",
			request: Request{
				UserUUID: "user-uuid-123",
				Name:     "",
				Email:    "user@example.com",
				Username: "johndoe",
			},
			want: false,
			wantErr: validationErrors{
				"name": "required_field",
			},
		},
		{
			name: "invalid request with empty email",
			request: Request{
				UserUUID: "user-uuid-123",
				Name:     "John Doe",
				Email:    "",
				Username: "johndoe",
			},
			want: false,
			wantErr: validationErrors{
				"email": "required_field",
			},
		},
		{
			name: "invalid request with empty username",
			request: Request{
				UserUUID: "user-uuid-123",
				Name:     "John Doe",
				Email:    "user@example.com",
				Username: "",
			},
			want: false,
			wantErr: validationErrors{
				"username": "required_field",
			},
		},
		{
			name: "invalid request with multiple errors",
			request: Request{
				UserUUID: "",
				Name:     "",
				Email:    "",
				Username: "",
			},
			want: false,
			wantErr: validationErrors{
				"uuid":     "required_field",
				"name":     "required_field",
				"email":    "required_field",
				"username": "required_field",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := tt.request.Validate()
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantErr, gotErr)
		})
	}
}

