package resource

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

func TestCommon(t *testing.T) {
	t.Parallel()

	since := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

	for name, tt := range map[string]struct {
		status string
		want   kind.Status
		err    bool
	}{
		"a kind's status is read for what every kind shares": {
			status: `{"state":"running","expected":"running","reason":"","since":"2026-10-06T12:00:00Z","uptime":42}`,
			want:   kind.Status{State: "running", Expected: "running", Since: since},
		},
		"no status is the zero status": {status: ``},
		"and so is null":               {status: `null`},
		"what is not a status is an error": {
			status: `["running"]`,
			err:    true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := Common(json.RawMessage(tt.status))
			if tt.err {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWithCommon(t *testing.T) {
	t.Parallel()

	since := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

	t.Run("what every kind shares is written, and the kind's own fields are kept", func(t *testing.T) {
		t.Parallel()

		got, err := WithCommon(
			json.RawMessage(`{"state":"running","reason":"it was","uptime":42}`),
			kind.Status{State: "stopped", Expected: "stopped", Since: since},
		)
		require.NoError(t, err)

		assert.JSONEq(t, `{"state":"stopped","expected":"stopped","since":"2026-10-06T12:00:00Z","uptime":42}`, string(got), "a reason that is no longer is cleared")
	})

	t.Run("a resource with no status is given one", func(t *testing.T) {
		t.Parallel()

		got, err := WithCommon(nil, kind.Status{State: "pending"})
		require.NoError(t, err)

		assert.JSONEq(t, `{"state":"pending"}`, string(got))
	})

	t.Run("a status that is not an object cannot be written into", func(t *testing.T) {
		t.Parallel()

		_, err := WithCommon(json.RawMessage(`"running"`), kind.Status{State: "stopped"})

		assert.Error(t, err)
	})
}

func TestMerge(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		recorded string
		observed string
		want     string
	}{
		"what a node says of a kind's own fields is taken": {
			recorded: `{"state":"running","uptime":1,"services":["web"]}`,
			observed: `{"state":"running","uptime":2,"services":[]}`,
			want:     `{"state":"running","uptime":2,"services":[]}`,
		},
		"and what it does not mention is kept": {
			recorded: `{"state":"running","output":"compose said so","uptime":1}`,
			observed: `{"state":"running","uptime":2}`,
			want:     `{"state":"running","output":"compose said so","uptime":2}`,
		},
		"but what every kind shares is never simply what it said": {
			recorded: `{"state":"stopping","expected":"stopped","reason":"","since":"2026-10-06T12:00:00Z"}`,
			observed: `{"state":"running","expected":"running","reason":"it is up","since":"2026-10-06T13:00:00Z","observed_at":"2026-10-06T13:00:00Z"}`,
			want:     `{"state":"stopping","expected":"stopped","reason":"","since":"2026-10-06T12:00:00Z"}`,
		},
		"a resource with no status yet takes the kind's fields": {
			recorded: ``,
			observed: `{"state":"running","uptime":2}`,
			want:     `{"uptime":2}`,
		},
		"and an observation that says nothing changes nothing": {
			recorded: `{"state":"running","uptime":1}`,
			observed: `null`,
			want:     `{"state":"running","uptime":1}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := Merge(json.RawMessage(tt.recorded), json.RawMessage(tt.observed))
			require.NoError(t, err)

			assert.JSONEq(t, tt.want, string(got))
		})
	}

	t.Run("a status that is not an object is an error", func(t *testing.T) {
		t.Parallel()

		_, err := Merge(json.RawMessage(`{"state":"running"}`), json.RawMessage(`[1]`))

		assert.Error(t, err)
	})
}

func TestRecord(t *testing.T) {
	t.Parallel()

	t.Run("its common status is read and written in place", func(t *testing.T) {
		t.Parallel()

		r := Record{Raw: kind.Raw{Kind: "fan", Status: json.RawMessage(`{"state":"stopped","speed":3}`)}}

		common, err := r.Common()
		require.NoError(t, err)
		assert.Equal(t, kind.State("stopped"), common.State)

		common.State = "starting"
		common.Expected = "running"
		require.NoError(t, r.SetCommon(common))

		assert.JSONEq(t, `{"state":"starting","expected":"running","speed":3}`, string(r.Status))
	})

	t.Run("a clone shares nothing with what it was made from", func(t *testing.T) {
		t.Parallel()

		r := Record{
			Raw: kind.Raw{
				Kind:     "fan",
				Metadata: kind.Metadata{UUID: "fan-uuid", Labels: map[string]string{"room": "kitchen"}, Owners: []kind.Reference{{Kind: "house", UUID: "house-uuid"}}},
				Spec:     json.RawMessage(`{"blades":3}`),
				Status:   json.RawMessage(`{"state":"running"}`),
			},
			Pending: &Pending{Action: "stop", Payload: json.RawMessage(`{}`), IDs: []string{"command-1"}},
			Answer:  &kind.ResourceActedOn{ID: "command-0", Status: json.RawMessage(`{}`)},
		}

		clone := r.Clone()
		clone.Metadata.Labels["room"] = "hall"
		clone.Metadata.Owners[0].UUID = "another"
		clone.Spec[1] = 'X'
		clone.Status[1] = 'X'
		clone.Pending.IDs[0] = "command-2"
		clone.Pending.Payload[0] = 'X'
		clone.Answer.ID = "command-3"

		assert.Equal(t, "kitchen", r.Metadata.Labels["room"])
		assert.Equal(t, "house-uuid", r.Metadata.Owners[0].UUID)
		assert.JSONEq(t, `{"blades":3}`, string(r.Spec))
		assert.JSONEq(t, `{"state":"running"}`, string(r.Status))
		assert.Equal(t, []string{"command-1"}, r.Pending.IDs)
		assert.Equal(t, "{}", string(r.Pending.Payload))
		assert.Equal(t, "command-0", r.Answer.ID)
	})

	t.Run("a pending command is answered by any of its tries, and by nothing else", func(t *testing.T) {
		t.Parallel()

		pending := &Pending{Action: "start", IDs: []string{"first", "second"}}

		assert.True(t, pending.Answers("first"))
		assert.True(t, pending.Answers("second"))
		assert.False(t, pending.Answers("an earlier command"))
		assert.False(t, pending.Answers(""))
		assert.False(t, (*Pending)(nil).Answers("first"))
	})
}

func TestDiffers(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		a, b string
		want bool
	}{
		"a status said again is no change":                  {a: `{"state":"running","speed":1}`, b: `{"speed":1,"state":"running"}`, want: false},
		"nor is it when only when it was observed moved on": {a: `{"state":"running","observed_at":"2026-10-06T12:00:00Z"}`, b: `{"state":"running","observed_at":"2026-10-06T12:00:01Z"}`, want: false},
		"a state that moved is a change":                    {a: `{"state":"running"}`, b: `{"state":"stopped"}`, want: true},
		"and so is a kind's own field":                      {a: `{"state":"running","speed":1}`, b: `{"state":"running","speed":2}`, want: true},
		"and one that is new":                               {a: `{"state":"running"}`, b: `{"state":"running","reason":"why"}`, want: true},
		"nothing at all is no status":                       {a: ``, b: `null`, want: false},
		"and what cannot be read is a change":               {a: `{"state":"running"}`, b: `[1]`, want: true},
		"numbers are numbers, however large":                {a: `{"bytes":9007199254740993}`, b: `{"bytes":9007199254740992}`, want: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, Differs(json.RawMessage(tt.a), json.RawMessage(tt.b)))
		})
	}
}
