package domain

import "context"

type Mailer interface {
	SendMail(ctx context.Context, from string, to string, subject string, body []byte) error
}
