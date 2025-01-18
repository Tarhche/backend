package email

import (
	"bytes"
	"fmt"
	"net/smtp"

	"github.com/khanzadimahdi/testproject/domain"
)

type Config struct {
	Auth Auth
	Host string
	Port string
}

type Auth struct {
	Username string
	Password string
}

type client struct {
	config Config
	addr   string
}

var _ domain.Mailer = NewSMTP(Config{})

func NewSMTP(config Config) *client {
	return &client{
		config: config,
		addr:   fmt.Sprintf("%s:%s", config.Host, config.Port),
	}
}

func (s *client) SendMail(from string, to string, subject string, body []byte) error {
	auth := smtp.PlainAuth("", s.config.Auth.Username, s.config.Auth.Password, s.config.Host)

	var msg bytes.Buffer

	mimeHeaders := "MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\n\n"

	if _, err := msg.WriteString(fmt.Sprintf("From: %s\nTo: %s\nSubject: %s\n%s", from, to, subject, mimeHeaders)); err != nil {
		return err
	}

	if _, err := msg.Write(body); err != nil {
		return err
	}

	return smtp.SendMail(s.addr, auth, from, []string{to}, msg.Bytes())
}
