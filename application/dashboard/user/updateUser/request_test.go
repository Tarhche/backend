package updateuser

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
				UserUUID: "user-uuid-123",
				Email:    "user@example.com",
				Name:     "John Doe",
				Avatar:   "avatar.jpg",
				Username: "johndoe",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "valid request with empty optional fields",
			request: Request{
				UserUUID: "user-uuid-123",
				Email:    "user@example.com",
				Name:     "John Doe",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "invalid request with empty user uuid",
			request: Request{
				UserUUID: "",
				Email:    "user@example.com",
				Name:     "John Doe",
			},
			want: domain.ValidationErrors{
				"uuid": "required_field",
			},
		},
		{
			name: "invalid request with empty email",
			request: Request{
				UserUUID: "user-uuid-123",
				Email:    "",
				Name:     "John Doe",
			},
			want: domain.ValidationErrors{
				"email": "required_field",
			},
		},
		{
			name: "invalid request with empty name",
			request: Request{
				UserUUID: "user-uuid-123",
				Email:    "user@example.com",
				Name:     "",
			},
			want: domain.ValidationErrors{
				"name": "required_field",
			},
		},
		{
			name: "invalid request with multiple errors",
			request: Request{
				UserUUID: "",
				Email:    "",
				Name:     "",
			},
			want: domain.ValidationErrors{
				"uuid":  "required_field",
				"email": "required_field",
				"name":  "required_field",
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
