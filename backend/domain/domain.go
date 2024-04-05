package domain

import "errors"

var (
	ErrNotExists = errors.New("not exists")
)

type Mailer interface {
	SendMail(from string, to string, subject string, body []byte) error
}
