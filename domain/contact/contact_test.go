package contact

import (
	"testing"
	"time"
)

func TestMessage_IsRead(t *testing.T) {
	if (Message{}).IsRead() {
		t.Error("a message with a zero ReadAt should not be read")
	}

	if !(Message{ReadAt: time.Now()}).IsRead() {
		t.Error("a message with a ReadAt should be read")
	}
}
