package commandbus

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBus(t *testing.T) {
	b := New()

	var counter int

	payload := make([]byte, 100)
	if _, err := rand.Read(payload); err != nil {
		t.Fatal("unexpected error", err)
	}

	command := &FakeCommand{Payload: payload}

	handler := func(command any) {
		counter++

		cmd, ok := command.(*FakeCommand)

		assert.True(t, ok, "invalid command")
		assert.Equal(t, payload, cmd.Payload, "command payload is not valid")
	}

	b.Register(&FakeCommand{}, HandlerFunc(handler))
	b.Execute(command)

	if counter != 1 {
		t.Errorf("command handler should be invoked once but invoked %d", counter)
	}
}

type FakeCommand struct {
	Payload []byte
}
