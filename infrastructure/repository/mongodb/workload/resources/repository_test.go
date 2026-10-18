package resources

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// TestValueOf holds a status, stored and read back, to what it was written as:
// a container's, whose state is the kind's and whose docker_state, beside it,
// is docker's, keeps both, each where it was.
func TestValueOf(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

	written := containerKind.Status{
		Status: kind.Status{State: containerKind.Stopped, Expected: containerKind.Running, Since: at, ObservedAt: at},
		Docker: &containerKind.Docker{
			ID:        "c0ffee",
			Name:      "web",
			Image:     "nginx:1.27",
			State:     "exited",
			Status:    "Exited (0) 2 minutes ago",
			Ports:     []containerKind.PortBinding{},
			Networks:  []string{"bridge"},
			Mounts:    []containerKind.Mount{},
			Labels:    map[string]string{"workload.managed": "true"},
			CreatedAt: at,
		},
	}

	raw, err := json.Marshal(written)
	require.NoError(t, err)

	stored, err := valueOf(raw)
	require.NoError(t, err)

	document := stored.Document()
	assert.Equal(t, "stopped", document.Lookup("state").StringValue(), "the kind's state, where the framework reads it")
	assert.Equal(t, "exited", document.Lookup("docker_state").StringValue(), "and docker's, beside it")
	assert.Equal(t, "Exited (0) 2 minutes ago", document.Lookup("docker_status").StringValue())

	read, err := jsonOf(stored)
	require.NoError(t, err)

	var kept containerKind.Status
	require.NoError(t, json.Unmarshal(read, &kept))
	assert.Equal(t, written, kept)
}

// TestValueOf_keys holds a kind's JSON, stored, to every key it is given, in
// the order it is given, whatever the key says: a label a person or an image
// gives may be named as extended JSON names a type, and is kept as a label,
// a document of its keys, all the same.
func TestValueOf_keys(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		written string

		// in is where an object of the keys is, as a path of them.
		in []string

		// read is what it is read back as, when not as it was written.
		read string
	}{
		"a label named as extended JSON names a date, of what is no date": {written: `{"labels":{"$date":"x"}}`, in: []string{"labels"}},
		"and of what is one":           {written: `{"labels":{"$date":"2026-10-06T12:00:00Z"}}`, in: []string{"labels"}},
		"one named as it names a long": {written: `{"labels":{"$numberLong":"5"}}`, in: []string{"labels"}},
		"an int":                       {written: `{"labels":{"$numberInt":"5"}}`, in: []string{"labels"}},
		"a double":                     {written: `{"labels":{"$numberDouble":"1.5"}}`, in: []string{"labels"}},
		"a decimal":                    {written: `{"labels":{"$numberDecimal":"1.5"}}`, in: []string{"labels"}},
		"an object id":                 {written: `{"labels":{"$oid":"5f0c8a7e9d1b2c3d4e5f6a7b"}}`, in: []string{"labels"}},
		"binary data":                  {written: `{"labels":{"$binary":"AAEC","$type":"00"}}`, in: []string{"labels"}},
		"a pattern":                    {written: `{"labels":{"$regularExpression":"^a"}}`, in: []string{"labels"}},
		"a timestamp":                  {written: `{"labels":{"$timestamp":"1"}}`, in: []string{"labels"}},
		"the least of anything":        {written: `{"labels":{"$minKey":"1"}}`, in: []string{"labels"}},
		"one among others, first, as JSON orders a map's keys":                  {written: `{"labels":{"$date":"x","com.example.tier":"web"}}`, in: []string{"labels"}},
		"a dot, a dollar alone, and nothing":                                    {written: `{"labels":{"":"none","$":"dollar","$$":"dollars","a.b":"dotted"}}`, in: []string{"labels"}},
		"deep in a list":                                                        {written: `{"ports":[{"$numberLong":"5"}]}`, in: []string{"ports", "0"}},
		"a key given twice, which holds what it is given last, as JSON is read": {written: `{"labels":{"a":"1","b":"2","a":"3"}}`, in: []string{"labels"}, read: `{"labels":{"a":"3","b":"2"}}`},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			stored, err := valueOf(json.RawMessage(tt.written))
			require.NoError(t, err)

			assert.Equal(t, bson.TypeEmbeddedDocument, stored.Document().Lookup(tt.in...).Type, "kept as a document of its keys")

			read, err := jsonOf(stored)
			require.NoError(t, err)

			want := tt.read
			if len(want) == 0 {
				want = tt.written
			}

			assert.Equal(t, want, string(read), "read back as it was written, its keys in their order")
		})
	}
}

// TestValueOf_numbers holds a number, stored, to what relaxed extended JSON
// reads it as, as it was stored before: an integer as an int32, or an int64
// beyond what that holds, or a double beyond both, and one with a fraction or
// an exponent as a double; and it is read back as that.
func TestValueOf_numbers(t *testing.T) {
	t.Parallel()

	for written, tt := range map[string]struct {
		kept any
		read string
	}{
		"0":                      {kept: int32(0), read: "0"},
		"-0":                     {kept: int32(0), read: "0"},
		"7":                      {kept: int32(7), read: "7"},
		"2147483647":             {kept: int32(math.MaxInt32), read: "2147483647"},
		"2147483648":             {kept: int64(math.MaxInt32 + 1), read: "2147483648"},
		"-2147483649":            {kept: int64(math.MinInt32 - 1), read: "-2147483649"},
		"9007199254740993":       {kept: int64(1<<53 + 1), read: "9007199254740993"},
		"9223372036854775807":    {kept: int64(math.MaxInt64), read: "9223372036854775807"},
		"-9223372036854775808":   {kept: int64(math.MinInt64), read: "-9223372036854775808"},
		"9223372036854775808":    {kept: float64(1 << 63), read: "9.223372036854776E+18"},
		"18446744073709551615":   {kept: float64(math.MaxUint64), read: "1.8446744073709552E+19"},
		"0.5":                    {kept: 0.5, read: "0.5"},
		"2.0":                    {kept: 2.0, read: "2.0"},
		"1e3":                    {kept: 1000.0, read: "1000.0"},
		"1E-7":                   {kept: 1e-7, read: "1E-07"},
		"-2.5e-300":              {kept: -2.5e-300, read: "-2.5E-300"},
		"1.7976931348623157e308": {kept: math.MaxFloat64, read: "1.7976931348623157E+308"},
	} {
		t.Run(written, func(t *testing.T) {
			t.Parallel()

			raw := json.RawMessage(`{"n":` + written + `}`)

			stored, err := valueOf(raw)
			require.NoError(t, err)

			n := stored.Document().Lookup("n")

			switch kept := tt.kept.(type) {
			case int32:
				assert.Equal(t, kept, n.Int32())
			case int64:
				assert.Equal(t, kept, n.Int64())
			case float64:
				assert.Equal(t, kept, n.Double())
			}

			before, err := extendedValueOf(raw)
			require.NoError(t, err)
			assert.Equal(t, before.Type, stored.Type)
			assert.Equal(t, before.Value, stored.Value, "as it was stored before")

			read, err := jsonOf(stored)
			require.NoError(t, err)
			assert.Equal(t, `{"n":`+tt.read+`}`, string(read))
		})
	}
}

// TestValueOf_asBefore holds a kind's JSON that names no type as extended
// JSON does, which is all a kind writes but for a key somebody else gives it,
// to being stored byte for byte as it was before: what was stored before is
// what is stored now, and reads back as it does.
func TestValueOf_asBefore(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 6, 12, 30, 15, 123456789, time.UTC)
	retries := 3

	for name, written := range map[string]any{
		"a container's spec": containerKind.Spec{
			VM:      stackKind.VMChoice{UUID: "vm-uuid", Resources: &stackKind.Resources{CPUs: 2, Memory: 2 << 30, Disk: 20 << 30}},
			Name:    "web",
			Image:   "nginx:1.27",
			Command: []string{"nginx", "-g", "daemon off;"},
			Env:     []string{"MODE=production", "EMPTY=", "QUOTE=\"a\"", "UNICODE=ک"},
			Ports:   []containerKind.PortBinding{{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"}},
			Aliases: map[string][]string{"shop_front.net": {"web", "www"}},
			CPUs:    0.5,
			Memory:  1<<62 + 1,
		},
		"a container's status": containerKind.Status{
			Status: kind.Status{State: containerKind.Running, Expected: containerKind.Running, Since: at, ObservedAt: at},
			Docker: &containerKind.Docker{ID: "c0ffee", Name: "web", State: "running", Ports: []containerKind.PortBinding{}, Networks: []string{}, Mounts: []containerKind.Mount{}, Labels: map[string]string{"workload.managed": "true", "org.opencontainers.image.title": "web"}, CreatedAt: at},
		},
		"a network's spec": networkKind.Spec{Name: "front", Driver: "bridge", Labels: map[string]string{"com.example.tier": "web", "": "nothing"}},
		"a stack's spec":   stackKind.Spec{VM: stackKind.VMChoice{UUID: "vm-uuid"}, Compose: "services:\n  web:\n    image: nginx\n    environment:\n      $date: x\n"},
		"a task's spec":    taskKind.Spec{Image: "python:3.13", Environment: []string{"A=1"}, Limits: taskKind.Limits{CPU: 0.25, Memory: 512 << 20, Disk: 1 << 30}, TTL: time.Minute, MaxRetries: &retries},
		"a vm's status": vmKind.Status{
			Status: kind.Status{State: vmKind.Running, Expected: vmKind.Running, Since: at},
			Stats:  &vmKind.Stats{CPUPercent: 12.345, MemoryUsed: 1 << 40, MemoryLimit: math.MaxInt64, SampledAt: at},
		},
		"nothing at all":  nil,
		"empties":         map[string]any{"object": map[string]any{}, "list": []any{}, "string": "", "null": nil, "false": false},
		"a list of lists": []any{[]any{}, []any{1, "two", 3.5, nil, true, []any{map[string]any{"deep": "er"}}}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			raw, err := json.Marshal(written)
			require.NoError(t, err)

			stored, err := valueOf(raw)
			require.NoError(t, err)

			before, err := extendedValueOf(raw)
			require.NoError(t, err)

			assert.Equal(t, before.Type, stored.Type)
			assert.Equal(t, before.Value, stored.Value, "stored byte for byte as before")
		})
	}
}

// TestValueOf_unstorable holds what cannot be kept as it is to being
// resource.ErrUnstorable: writing it again would fail again.
func TestValueOf_unstorable(t *testing.T) {
	t.Parallel()

	for name, written := range map[string]string{
		"a key with a NUL in it":                 `{"labels":{"a\u0000b":"c"}}`,
		"a number beyond what a double holds":    `{"n":1e400}`,
		"an integer of as many digits":           `{"n":1` + strings.Repeat("0", 400) + `}`,
		"what is cut short":                      `{"labels":{`,
		"what is no JSON":                        `{labels}`,
		"more than one value":                    `{"a":1} {"b":2}`,
		"a value with more after it than it has": `{"a":1}}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := valueOf(json.RawMessage(written))
			assert.ErrorIs(t, err, resource.ErrUnstorable)
		})
	}
}

// TestUnstorable holds what writing a document failed with to being said to
// be of the document (resource.ErrUnstorable) only when it is: one the driver
// cannot write, or the database refuses as too large or nested too deep.
func TestUnstorable(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		err  error
		kept bool
	}{
		"a document the driver cannot write":    {err: mongo.MarshalError{Value: "a document", Err: errors.New("BSON element key cannot contain null bytes")}},
		"one beyond what a message holds":       {err: driver.ErrDocumentTooLarge},
		"one beyond what a document holds":      {err: mongo.WriteException{WriteErrors: mongo.WriteErrors{{Code: 10334, Message: "object to insert too large"}}}},
		"one nested deeper than it nests one":   {err: mongo.WriteException{WriteErrors: mongo.WriteErrors{{Code: 15, Message: "cannot insert document because it exceeds 180 levels of nesting"}}}},
		"and deeper still":                      {err: mongo.CommandError{Code: 15, Message: "BSONObj exceeds maximum nested object depth"}},
		"but not the database gone for a while": {err: errors.New("connection refused"), kept: true},
		"nor a write cut short":                 {err: context.DeadlineExceeded, kept: true},
		"nor a write refused for another cause": {err: mongo.CommandError{Code: 11600, Message: "interrupted at shutdown"}, kept: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := unstorable(tt.err)
			assert.ErrorContains(t, err, tt.err.Error(), "what it failed with is said")
			assert.Equal(t, !tt.kept, errors.Is(err, resource.ErrUnstorable))
		})
	}
}

// extendedValueOf is a kind's JSON as a value to store as it was stored
// before valueOf kept every key as it is: read as relaxed extended JSON.
func extendedValueOf(raw json.RawMessage) (bson.RawValue, error) {
	if len(raw) == 0 {
		raw = json.RawMessage("null")
	}

	var value struct {
		V bson.RawValue `bson:"v"`
	}

	if err := bson.UnmarshalExtJSON(append(append([]byte(`{"v":`), raw...), '}'), false, &value); err != nil {
		return bson.RawValue{}, err
	}

	return value.V, nil
}
