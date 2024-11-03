package memory

import (
	"bytes"
	"context"
	"crypto/rand"
	"testing"

	"github.com/khanzadimahdi/testproject/domain"
)

func TestBus(t *testing.T) {
	b := NewSyncPublishSubscriber()

	payload := make([]byte, 100)
	if _, err := rand.Read(payload); err != nil {
		t.Fatal("unexpected error", err)
	}

	command := &FakeEvent{Payload: payload}

	var counter1 int
	handler1 := func(payload []byte) error {
		counter1++

		if !bytes.Equal(payload, command.Payload) {
			t.Error("command payload is not valid")
		}

		return nil
	}

	var counter2 int
	handler2 := func(payload []byte) error {
		counter2++

		if !bytes.Equal(payload, command.Payload) {
			t.Error("command payload is not valid")
		}

		return nil
	}

	ctx := context.Background()

	b.Subscribe(ctx, "1", "test-subject", domain.MessageHandlerFunc(handler1))
	b.Subscribe(ctx, "2", "test-subject", domain.MessageHandlerFunc(handler2))
	b.Publish(ctx, "test-subject", command.Payload)

	if counter1 != 1 {
		t.Errorf("event handler-1 should be invoked once but invoked %d", counter1)
	}

	if counter2 != 1 {
		t.Errorf("event handler-2 should be invoked once but invoked %d", counter2)
	}
}

type FakeEvent struct {
	Payload []byte
}
