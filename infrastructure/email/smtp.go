package email

import (
	"bytes"
	"context"
	"fmt"
	"net/smtp"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel"
	oteltrace "go.opentelemetry.io/otel/trace"
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
	tracer oteltrace.Tracer
}

var _ domain.Mailer = NewSMTP(Config{})

func NewSMTP(config Config) *client {
	return &client{
		config: config,
		addr:   fmt.Sprintf("%s:%s", config.Host, config.Port),
		tracer: otel.Tracer("smtp"),
	}
}

func (s *client) SendMail(ctx context.Context, from string, to string, subject string, body []byte) error {
	_, span := s.tracer.Start(ctx, "smtp.send", oteltrace.WithSpanKind(oteltrace.SpanKindClient))
	defer span.End()

	var auth smtp.Auth
	if len(s.config.Auth.Username) > 0 || len(s.config.Auth.Password) > 0 {
		auth = smtp.PlainAuth("", s.config.Auth.Username, s.config.Auth.Password, s.config.Host)
	}

	var msg bytes.Buffer

	mimeHeaders := "MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\n\n"

	if _, err := msg.WriteString(fmt.Sprintf("From: %s\nTo: %s\nSubject: %s\n%s", from, to, subject, mimeHeaders)); err != nil {
		return trace.RecordError(span, err)
	}

	if _, err := msg.Write(body); err != nil {
		return trace.RecordError(span, err)
	}

	return trace.RecordError(span, smtp.SendMail(s.addr, auth, from, []string{to}, msg.Bytes()))
}
