package eventbus

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestBus(t *testing.T) {
	b := New()

	payload := make([]byte, 100)
	if _, err := rand.Read(payload); err != nil {
		t.Fatal("unexpected error", err)
	}

	command := &FakeEvent{Payload: payload}

	var counter1 int
	handler1 := func(command any) {
		counter1++

		cmd, ok := command.(*FakeEvent)
		if !ok {
			t.Error("invalid command")
		}

		if !bytes.Equal(payload, cmd.Payload) {
			t.Error("command payload is not valid")
		}
	}

	var counter2 int
	handler2 := func(command any) {
		counter2++

		cmd, ok := command.(*FakeEvent)
		if !ok {
			t.Error("invalid command")
		}

		if !bytes.Equal(payload, cmd.Payload) {
			t.Error("command payload is not valid")
		}
	}

	b.Subscribe(&FakeEvent{}, HandlerFunc(handler1))
	b.Subscribe(&FakeEvent{}, HandlerFunc(handler2))
	b.Publish(command)

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
