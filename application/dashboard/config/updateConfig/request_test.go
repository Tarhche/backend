package updateConfig

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
				UserDefaultRoles:    []string{"user", "editor"},
				DefaultLanguageCode: "EN",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "valid request with single role",
			request: Request{
				UserDefaultRoles:    []string{"user"},
				DefaultLanguageCode: "FA",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "invalid request with empty user default roles",
			request: Request{
				UserDefaultRoles:    []string{},
				DefaultLanguageCode: "EN",
			},
			want: domain.ValidationErrors{
				"user_default_roles": "required_field",
			},
		},
		{
			name: "invalid request with nil user default roles",
			request: Request{
				UserDefaultRoles:    nil,
				DefaultLanguageCode: "EN",
			},
			want: domain.ValidationErrors{
				"user_default_roles": "required_field",
			},
		},
		{
			name: "invalid request with empty default language",
			request: Request{
				UserDefaultRoles:    []string{"user"},
				DefaultLanguageCode: "",
			},
			want: domain.ValidationErrors{
				"default_language_code": "required_field",
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
