package refresh

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
				Token: "valid-refresh-token",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "invalid request with empty token",
			request: Request{
				Token: "",
			},
			want: domain.ValidationErrors{
				"token": "required_field",
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

