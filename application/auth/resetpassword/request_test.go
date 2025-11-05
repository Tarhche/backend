package resetpassword

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
				Token:    "valid-token",
				Password: "newpassword123",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "invalid request with empty token",
			request: Request{
				Token:    "",
				Password: "newpassword123",
			},
			want: domain.ValidationErrors{
				"token": "required_field",
			},
		},
		{
			name: "invalid request with empty password",
			request: Request{
				Token:    "valid-token",
				Password: "",
			},
			want: domain.ValidationErrors{
				"password": "required_field",
			},
		},
		{
			name: "invalid request with both empty",
			request: Request{
				Token:    "",
				Password: "",
			},
			want: domain.ValidationErrors{
				"token":    "required_field",
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

