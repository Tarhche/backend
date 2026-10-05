// Package reply is what every answer a node gives the control plane's
// requests does the same way: reading the payload it was asked with, and
// fitting what it answers into one NATS message.
package reply

import (
	"encoding/json"
	"fmt"

	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

// budget is how much of a reply its result may take: what one NATS message
// carries, less room for the envelope around the result.
const budget = noderequest.MaxReplyBytes - 4<<10

// Decode reads the payload a request was asked with. A request that carries
// none is asked with the payload's zero value.
func Decode(payload json.RawMessage, into any) error {
	if len(payload) == 0 {
		return nil
	}

	if err := json.Unmarshal(payload, into); err != nil {
		return Invalid("the payload cannot be read: %v", err)
	}

	return nil
}

// Invalid is a request refused as it stood, for the reason given.
func Invalid(format string, args ...any) error {
	return &noderequest.Error{Code: noderequest.CodeInvalid, Message: fmt.Sprintf(format, args...)}
}

// Required refuses a request that leaves out what it has to say.
func Required(field string, value string) error {
	if len(value) > 0 {
		return nil
	}

	return Invalid("%s is required", field)
}

// First is as many of items as fit in a reply, counted from the first, and
// whether any had to be left out: a listing keeps its first items.
func First[T any](items []T) ([]T, bool) {
	return fit(items, false)
}

// Last is as many of items as fit in a reply, counted back from the last, and
// whether any had to be left out: a log keeps its last lines.
func Last[T any](items []T) ([]T, bool) {
	return fit(items, true)
}

func fit[T any](items []T, last bool) ([]T, bool) {
	if size(items) <= budget {
		return items, false
	}

	// the most items that fit, found by halving: what fits shrinks as fewer
	// are kept.
	low, high := 0, len(items)
	for low < high {
		middle := (low + high + 1) / 2

		if size(window(items, middle, last)) <= budget {
			low = middle
		} else {
			high = middle - 1
		}
	}

	return window(items, low, last), true
}

func window[T any](items []T, count int, last bool) []T {
	if last {
		return items[len(items)-count:]
	}

	return items[:count]
}

func size[T any](items []T) int {
	encoded, err := json.Marshal(items)
	if err != nil {
		return 0
	}

	return len(encoded)
}
