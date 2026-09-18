package impersonateuser

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain"
)

func TestRequest_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		request Request
		want    domain.ValidationErrors
	}{
		{
			name: "valid request",
			request: Request{
				UserUUID:         "user-uuid",
				ImpersonatorUUID: "impersonator-uuid",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "nobody to be seen as",
			request: Request{
				ImpersonatorUUID: "impersonator-uuid",
			},
			want: domain.ValidationErrors{
				"uuid": "required_field",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.request.Validate())
		})
	}
}
