package createMessage

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain"
)

func TestRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request Request
		want    domain.ValidationErrors
	}{
		{
			name: "valid request with an email",
			request: Request{
				Subject: "a subject",
				Body:    "a body",
				Email:   "user@example.com",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "valid request with a phone number",
			request: Request{
				Subject: "a subject",
				Body:    "a body",
				Phone:   "09123456789",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "valid request with both",
			request: Request{
				Subject: "a subject",
				Body:    "a body",
				Email:   "user@example.com",
				Phone:   "1234",
			},
			want: domain.ValidationErrors{},
		},
		{
			name: "missing subject and body",
			request: Request{
				Email: "user@example.com",
			},
			want: domain.ValidationErrors{
				"subject": "required_field",
				"body":    "required_field",
			},
		},
		{
			name: "blank subject and body",
			request: Request{
				Subject: "   ",
				Body:    "\n\t",
				Email:   "user@example.com",
			},
			want: domain.ValidationErrors{
				"subject": "required_field",
				"body":    "required_field",
			},
		},
		{
			name: "neither an email nor a phone number",
			request: Request{
				Subject: "a subject",
				Body:    "a body",
			},
			want: domain.ValidationErrors{
				"email": "email_or_phone_required",
				"phone": "email_or_phone_required",
			},
		},
		{
			name: "malformed email",
			request: Request{
				Subject: "a subject",
				Body:    "a body",
				Email:   "not-an-email",
			},
			want: domain.ValidationErrors{
				"email": "invalid_email",
			},
		},
		{
			name: "malformed phone number",
			request: Request{
				Subject: "a subject",
				Body:    "a body",
				Phone:   "12-3",
			},
			want: domain.ValidationErrors{
				"phone": "invalid_phone_number",
			},
		},
		{
			name: "too short phone number",
			request: Request{
				Subject: "a subject",
				Body:    "a body",
				Phone:   "123",
			},
			want: domain.ValidationErrors{
				"phone": "invalid_phone_number",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.request.Validate())
		})
	}
}
