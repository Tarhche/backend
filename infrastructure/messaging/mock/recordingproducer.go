package mock

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/khanzadimahdi/testproject/domain"
)

// Produced is one message a RecordingProducer was given.
type Produced struct {
	Subject string
	Payload []byte
}

// RecordingProducer keeps every message it is given, so a test that is about
// what was asked of the nodes can read it back rather than set up an
// expectation for each one.
type RecordingProducer struct {
	lock     sync.Mutex
	produced []Produced

	// Fail, when set, is what every message reports instead of being kept.
	Fail error
}

var _ domain.Producer = &RecordingProducer{}

func (p *RecordingProducer) Produce(_ context.Context, subject string, payload []byte) error {
	if p.Fail != nil {
		return p.Fail
	}

	p.lock.Lock()
	defer p.lock.Unlock()

	p.produced = append(p.produced, Produced{Subject: subject, Payload: append([]byte(nil), payload...)})

	return nil
}

// Produced is every message given so far, in order.
func (p *RecordingProducer) Produced() []Produced {
	p.lock.Lock()
	defer p.lock.Unlock()

	return append([]Produced(nil), p.produced...)
}

// Subjects is the subject of every message given so far, in order, or nil
// when none was.
func (p *RecordingProducer) Subjects() []string {
	produced := p.Produced()
	if len(produced) == 0 {
		return nil
	}

	subjects := make([]string, len(produced))
	for i := range produced {
		subjects[i] = produced[i].Subject
	}

	return subjects
}

// Last decodes the last message given on subject into out, and reports whether
// there was one.
func (p *RecordingProducer) Last(subject string, out any) bool {
	produced := p.Produced()

	for i := len(produced) - 1; i >= 0; i-- {
		if produced[i].Subject == subject {
			return json.Unmarshal(produced[i].Payload, out) == nil
		}
	}

	return false
}

// Reset forgets what was given so far.
func (p *RecordingProducer) Reset() {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.produced = nil
}
