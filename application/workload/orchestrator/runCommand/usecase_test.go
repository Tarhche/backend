package runCommand

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/lock"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	messaging "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
)

// lights is a strategy that lights a lamp as bright as it is asked, and
// remembers what it was asked.
func lights(asked *[]string) executing {
	return func(_ context.Context, r kind.Resource[lampSpec, lampStatus], action string, payload any) (kind.Outcome[lampStatus], error) {
		*asked = append(*asked, fmt.Sprintf("%s %s %v", action, r.Metadata.UUID, payload))

		if action == "delete" {
			return kind.Outcome[lampStatus]{Status: lampStatus{Status: kind.Status{State: kind.Deleted}}}, nil
		}

		light := payload.(lightPayload)

		return kind.Outcome[lampStatus]{
			Status: lampStatus{Status: kind.Status{State: lit}, Brightness: light.Brightness},
			Output: "click",
		}, nil
	}
}

// results are the results said so far.
func results(t *testing.T, recorder *messaging.Recorder) []kind.Result {
	t.Helper()

	said, err := messaging.Produced[kind.Result](recorder, kind.ResultName)
	require.NoError(t, err)

	return said
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		command  func(t *testing.T) kind.Command
		execute  func(asked *[]string) executing
		ok       bool
		status   string
		reason   string
		output   string
		executed []string
	}{
		"a command is carried out, and what it left is said": {
			command:  func(t *testing.T) kind.Command { return aCommand(t, "lamp-1", "light", `{"brightness": 80}`) },
			execute:  lights,
			ok:       true,
			status:   `{"state":"lit","brightness":80}`,
			output:   "click",
			executed: []string{"light lamp-1 {80}"},
		},
		"an action asked with nothing is carried out with nothing": {
			command:  func(t *testing.T) kind.Command { return aCommand(t, "lamp-1", "delete", ``) },
			execute:  lights,
			ok:       true,
			status:   `{"state":"deleted"}`,
			executed: []string{"delete lamp-1 <nil>"},
		},
		"a command that failed is said to have, and why": {
			command: func(t *testing.T) kind.Command { return aCommand(t, "lamp-1", "light", `{"brightness": 80}`) },
			execute: func(asked *[]string) executing {
				return func(_ context.Context, r kind.Resource[lampSpec, lampStatus], action string, _ any) (kind.Outcome[lampStatus], error) {
					*asked = append(*asked, action+" "+r.Metadata.UUID)

					return kind.Outcome[lampStatus]{Output: "fzzt"}, errors.New("the bulb is broken")
				}
			},
			reason:   "the bulb is broken",
			output:   "fzzt",
			executed: []string{"light lamp-1"},
		},
		"a kind this node does not run is said not to be": {
			command: func(t *testing.T) kind.Command {
				c := aCommand(t, "lamp-1", "light", `{"brightness": 80}`)
				c.Kind = "kettle"

				return c
			},
			execute: lights,
			reason:  `unknown kind: this node runs no "kettle"`,
		},
		"an action the kind does not have is said not to be": {
			command: func(t *testing.T) kind.Command { return aCommand(t, "lamp-1", "explode", ``) },
			execute: lights,
			reason:  "unknown action",
		},
		"and so is a query sent as a command": {
			command: func(t *testing.T) kind.Command { return aCommand(t, "lamp-1", "state", ``) },
			execute: lights,
			reason:  "unknown action",
		},
		"a payload that cannot be read is said to be": {
			command: func(t *testing.T) kind.Command { return aCommand(t, "lamp-1", "light", `{"brightness": "very"}`) },
			execute: lights,
			reason:  "invalid payload",
		},
		"and so is one that is not valid, with why": {
			command: func(t *testing.T) kind.Command { return aCommand(t, "lamp-1", "light", `{"brightness": 500}`) },
			execute: lights,
			reason:  "brightness: out_of_range",
		},
		"a command that names no resource is carried out on none": {
			command: func(t *testing.T) kind.Command {
				c := aCommand(t, "", "light", `{"brightness": 80}`)

				return c
			},
			execute: lights,
			reason:  "invalid payload: the command names no lamp",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var executed []string
			recorder := &messaging.Recorder{}

			asked := tt.command(t)

			err := NewUseCase(running(t, tt.execute(&executed)), lock.New(), recorder).Execute(t.Context(), asked)
			require.NoError(t, err, "whatever came of it was said, and is not asked again")

			said := results(t, recorder)
			require.Len(t, said, 1, "one command, one result")

			result := said[0]

			assert.Equal(t, tt.ok, result.OK, "ok")
			assert.Contains(t, result.Reason, tt.reason, "reason")
			assert.Equal(t, tt.output, result.Output, "output")
			assert.Equal(t, tt.executed, executed, "executed")

			if len(tt.status) > 0 {
				assert.JSONEq(t, tt.status, string(result.Status), "status")
			} else {
				assert.Empty(t, result.Status, "status")
			}

			if !tt.ok {
				assert.NotEmpty(t, result.Reason, "a failure says why")
			}

			assert.Equal(t, asked.ID, result.ID, "a result is its command's")
			assert.Equal(t, asked.Kind, result.Kind)
			assert.Equal(t, asked.UUID, result.UUID)
			assert.Equal(t, asked.Action, result.Action)
			assert.Equal(t, asked.Node, result.Node)
			assert.Equal(t, asked.Attempt, result.Attempt)
			assert.WithinDuration(t, time.Now(), result.At, time.Minute, "it is stamped when it is said")
		})
	}

	t.Run("what came of a command that cannot be said is worth another delivery", func(t *testing.T) {
		t.Parallel()

		var executed []string
		recorder := &messaging.Recorder{Err: errors.New("nats is away")}

		err := NewUseCase(running(t, lights(&executed)), lock.New(), recorder).Execute(t.Context(), aCommand(t, "lamp-1", "light", `{"brightness": 80}`))

		assert.ErrorContains(t, err, "nats is away")
	})

	t.Run("and so is a kind this node does not run, said to nobody", func(t *testing.T) {
		t.Parallel()

		recorder := &messaging.Recorder{Err: errors.New("nats is away")}

		c := aCommand(t, "lamp-1", "light", `{"brightness": 80}`)
		c.Kind = "kettle"

		assert.Error(t, NewUseCase(kind.NewRegistry[kind.NodeBinding](), lock.New(), recorder).Execute(t.Context(), c))
	})

	t.Run("a node going away has not failed a command, and leaves it to be carried out again", func(t *testing.T) {
		t.Parallel()

		recorder := &messaging.Recorder{}
		started := make(chan struct{})

		waits := func(*[]string) executing {
			return func(ctx context.Context, _ kind.Resource[lampSpec, lampStatus], _ string, _ any) (kind.Outcome[lampStatus], error) {
				close(started)
				<-ctx.Done()

				return kind.Outcome[lampStatus]{}, ctx.Err()
			}
		}

		ctx, cancel := context.WithCancel(t.Context())

		done := make(chan error, 1)
		go func() {
			done <- NewUseCase(running(t, waits(nil)), lock.New(), recorder).Execute(ctx, aCommand(t, "lamp-1", "light", `{"brightness": 80}`))
		}()

		<-started
		cancel()

		err := <-done
		assert.ErrorIs(t, err, context.Canceled)
		assert.Empty(t, results(t, recorder), "nothing is said of a command left halfway")
	})

	t.Run("nor does one that goes away while waiting its turn hold anything", func(t *testing.T) {
		t.Parallel()

		var executed []string
		recorder := &messaging.Recorder{}
		locks := lock.New()

		release, err := locks.Lock(t.Context(), "lamp-1")
		require.NoError(t, err)
		defer release()

		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		defer cancel()

		err = NewUseCase(running(t, lights(&executed)), locks, recorder).Execute(ctx, aCommand(t, "lamp-1", "light", `{"brightness": 80}`))

		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Empty(t, executed)
		assert.Empty(t, results(t, recorder))
	})
}

// journal is what happened, in the order it did, from the strategy's side
// and the producer's.
type journal struct {
	lock    sync.Mutex
	entries []string
}

func (j *journal) write(entry string) {
	j.lock.Lock()
	defer j.lock.Unlock()

	j.entries = append(j.entries, entry)
}

func (j *journal) read() []string {
	j.lock.Lock()
	defer j.lock.Unlock()

	return append([]string(nil), j.entries...)
}

// journalled is a producer that writes what it is given into a journal.
type journalled struct {
	journal *journal
}

var _ domain.Producer = journalled{}

func (p journalled) Produce(_ context.Context, _ string, payload []byte) error {
	p.journal.write("said " + string(payload))

	return nil
}

func TestUseCase_Execute_Locks(t *testing.T) {
	t.Parallel()

	t.Run("commands to one resource take turns, each said before the next begins", func(t *testing.T) {
		t.Parallel()

		var inside, most atomic.Int32

		j := &journal{}

		takesTurns := func(_ context.Context, r kind.Resource[lampSpec, lampStatus], action string, payload any) (kind.Outcome[lampStatus], error) {
			now := inside.Add(1)
			defer inside.Add(-1)

			for {
				seen := most.Load()
				if now <= seen || most.CompareAndSwap(seen, now) {
					break
				}
			}

			j.write("began")
			time.Sleep(time.Millisecond)

			return kind.Outcome[lampStatus]{Status: lampStatus{Status: kind.Status{State: lit}}}, nil
		}

		useCase := NewUseCase(running(t, takesTurns), lock.New(), journalled{journal: j})

		var wg sync.WaitGroup
		for range 10 {
			wg.Go(func() {
				assert.NoError(t, useCase.Execute(t.Context(), aCommand(t, "lamp-1", "light", `{"brightness": 80}`)))
			})
		}

		wg.Wait()

		assert.Equal(t, int32(1), most.Load(), "one at a time")

		entries := j.read()
		require.Len(t, entries, 20)

		for i := 0; i < len(entries); i += 2 {
			assert.Equal(t, "began", entries[i], "a command begins only once the last one's result is said")
			assert.Contains(t, entries[i+1], "said ", "and its own result is said before the next begins")
		}
	})

	t.Run("commands to different resources do not wait for each other", func(t *testing.T) {
		t.Parallel()

		recorder := &messaging.Recorder{}
		holding := make(chan struct{})
		letGo := make(chan struct{})

		waitsForOne := func(_ context.Context, r kind.Resource[lampSpec, lampStatus], _ string, _ any) (kind.Outcome[lampStatus], error) {
			if r.Metadata.UUID == "lamp-1" {
				close(holding)
				<-letGo
			}

			return kind.Outcome[lampStatus]{Status: lampStatus{Status: kind.Status{State: lit}}}, nil
		}

		useCase := NewUseCase(running(t, waitsForOne), lock.New(), recorder)

		first := make(chan error, 1)
		go func() {
			first <- useCase.Execute(t.Context(), aCommand(t, "lamp-1", "light", `{"brightness": 80}`))
		}()

		<-holding

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		require.NoError(t, useCase.Execute(ctx, aCommand(t, "lamp-2", "light", `{"brightness": 80}`)), "lamp-2 is lit while lamp-1 is still being lit")

		said := results(t, recorder)
		require.Len(t, said, 1)
		assert.Equal(t, "lamp-2", said[0].UUID)

		close(letGo)
		require.NoError(t, <-first)
		assert.Len(t, results(t, recorder), 2)
	})
}
