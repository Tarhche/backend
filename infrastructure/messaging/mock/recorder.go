package mock

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/khanzadimahdi/testproject/domain"
)

// Message is one message a Recorder was given.
type Message struct {
	Subject string
	Payload []byte
}

// Recorder is a producer that keeps what it is given, so a test can read back
// what was said rather than say in advance what will be. Err, when it is set,
// is what producing answers, which is how NATS being away looks.
type Recorder struct {
	lock     sync.Mutex
	messages []Message

	Err error
}

var _ domain.Producer = &Recorder{}

func (r *Recorder) Produce(ctx context.Context, subject string, payload []byte) error {
	r.lock.Lock()
	defer r.lock.Unlock()

	if r.Err != nil {
		return r.Err
	}

	r.messages = append(r.messages, Message{Subject: subject, Payload: append([]byte(nil), payload...)})

	return nil
}

// Messages is everything produced, in order.
func (r *Recorder) Messages() []Message {
	r.lock.Lock()
	defer r.lock.Unlock()

	return append([]Message(nil), r.messages...)
}

// Subjects is the subject of everything produced, in order.
func (r *Recorder) Subjects() []string {
	var subjects []string
	for _, message := range r.Messages() {
		subjects = append(subjects, message.Subject)
	}

	return subjects
}

// Last decodes the last message produced on subject into out, and reports
// whether there was one.
func (r *Recorder) Last(subject string, out any) bool {
	messages := r.Messages()

	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Subject == subject {
			return json.Unmarshal(messages[i].Payload, out) == nil
		}
	}

	return false
}

// Reset forgets what was produced so far.
func (r *Recorder) Reset() {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.messages = nil
}

// Produced is every message produced on subject, decoded as T.
func Produced[T any](r *Recorder, subject string) ([]T, error) {
	var decoded []T

	for _, message := range r.Messages() {
		if message.Subject != subject {
			continue
		}

		var event T
		if err := json.Unmarshal(message.Payload, &event); err != nil {
			return nil, err
		}

		decoded = append(decoded, event)
	}

	return decoded, nil
}
