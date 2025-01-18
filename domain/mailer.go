package domain

type Mailer interface {
	SendMail(from string, to string, subject string, body []byte) error
}
