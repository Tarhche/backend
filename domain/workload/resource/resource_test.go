package resource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// TestUnreadable holds what is said of what was read or written, which fails
// again however often it is tried, apart from what a database that did not
// answer, or answered otherwise this time, says.
func TestUnreadable(t *testing.T) {
	t.Parallel()

	var typed map[string]string
	notAMap := json.Unmarshal([]byte(`5`), &typed)

	for name, tt := range map[string]struct {
		err        error
		unreadable bool
	}{
		"json that is not json":                  {err: json.Unmarshal([]byte(`{`), &typed), unreadable: true},
		"json that is not what it is read as":    {err: fmt.Errorf("the status cannot be read: %w", notAMap), unreadable: true},
		"what has no json":                       {err: &json.UnsupportedValueError{Str: "NaN"}, unreadable: true},
		"a payload its kind cannot read":         {err: fmt.Errorf("%w: speed", kind.ErrInvalidPayload), unreadable: true},
		"of a kind that is none":                 {err: kind.ErrUnknownKind, unreadable: true},
		"a record its repository cannot keep":    {err: fmt.Errorf("%w: a key with a NUL in it", ErrUnstorable), unreadable: true},
		"but not a database that did not answer": {err: errors.New("connection refused")},
		"nor one that took too long":             {err: context.DeadlineExceeded},
		"nor a record written since it was read": {err: ErrConflict},
		"nor one that is gone":                   {err: domain.ErrNotExists},
		"nor nothing":                            {},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.unreadable, Unreadable(tt.err))
		})
	}
}
