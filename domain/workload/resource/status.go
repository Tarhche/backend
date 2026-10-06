package resource

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// commonFields are the names of the fields of a status every kind shares, as
// they are written: a kind's own status embeds kind.Status, so they are among
// its top-level fields, and everything else at that level is the kind's.
var commonFields = func() map[string]bool {
	moment := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

	every, err := json.Marshal(kind.Status{State: "x", Expected: "x", Reason: "x", Since: moment, ObservedAt: moment})
	if err != nil {
		panic(err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(every, &fields); err != nil {
		panic(err)
	}

	names := make(map[string]bool, len(fields))
	for name := range fields {
		names[name] = true
	}

	return names
}()

// Common reads the part of a kind's status every kind shares, which is the
// same whatever the kind: its own fields are left unread. No status at all is
// the zero status.
func Common(status json.RawMessage) (kind.Status, error) {
	var common kind.Status

	if absent(status) {
		return common, nil
	}

	if err := json.Unmarshal(status, &common); err != nil {
		return kind.Status{}, fmt.Errorf("the status cannot be read: %w", err)
	}

	return common, nil
}

// WithCommon is status with the part every kind shares written as common
// says, and the kind's own fields as they were.
func WithCommon(status json.RawMessage, common kind.Status) (json.RawMessage, error) {
	fields, err := fieldsOf(status)
	if err != nil {
		return nil, err
	}

	for name := range commonFields {
		delete(fields, name)
	}

	written, err := json.Marshal(common)
	if err != nil {
		return nil, err
	}

	var shared map[string]json.RawMessage
	if err := json.Unmarshal(written, &shared); err != nil {
		return nil, err
	}

	maps.Copy(fields, shared)

	return json.Marshal(fields)
}

// Merge is recorded with the kind's own fields that observed says, each in
// place of what recorded said of it. The fields every kind shares are
// recorded's: what they become is the framework's to decide, by the kind's
// machine, and never simply what a node said. A kind's own field observed
// does not mention is kept as it was recorded, so a field only the control
// plane fills in, written with omitempty, outlives what a node reports.
func Merge(recorded json.RawMessage, observed json.RawMessage) (json.RawMessage, error) {
	fields, err := fieldsOf(recorded)
	if err != nil {
		return nil, err
	}

	said, err := fieldsOf(observed)
	if err != nil {
		return nil, err
	}

	for name, value := range said {
		if !commonFields[name] {
			fields[name] = value
		}
	}

	return json.Marshal(fields)
}

// fieldsOf is a status's top-level fields. No status at all has none.
func fieldsOf(status json.RawMessage) (map[string]json.RawMessage, error) {
	fields := make(map[string]json.RawMessage)

	if absent(status) {
		return fields, nil
	}

	if err := json.Unmarshal(status, &fields); err != nil {
		return nil, fmt.Errorf("the status is not an object: %w", err)
	}

	if fields == nil {
		fields = make(map[string]json.RawMessage)
	}

	return fields, nil
}

// absent reports whether raw says nothing at all.
func absent(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)

	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}
