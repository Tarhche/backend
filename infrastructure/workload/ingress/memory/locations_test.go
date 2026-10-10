package memory

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/ingress"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

const (
	running  kind.State = "running"
	stopped  kind.State = "stopped"
	stopping kind.State = "stopping"
)

// clock is a time the test moves.
type clock struct {
	lock sync.Mutex
	at   time.Time
}

func (c *clock) now() time.Time {
	c.lock.Lock()
	defer c.lock.Unlock()

	return c.at
}

func (c *clock) pass(d time.Duration) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.at = c.at.Add(d)
}

// on is locations that forget what has gone unheard for a minute, on a
// clock the test moves, which the nodes' clocks agree with.
func on(t *testing.T) (*Locations, *clock) {
	t.Helper()

	c := &clock{at: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)}

	l := NewLocations(time.Minute)
	l.now = c.now

	return l, c
}

// task is what a node said of a task at a moment: running, under a slug,
// letting the ingress in to ports.
func task(uuid string, slug string, node string, at time.Time, ports ...port.Port) ingress.Heard {
	return ingress.Heard{Kind: "task", UUID: uuid, Slug: slug, Node: node, State: running, Ports: ports, At: at}
}

// doing is heard, doing state.
func doing(heard ingress.Heard, state kind.State) ingress.Heard {
	heard.State = state

	return heard
}

// found is where l finds the task a slug names, or why it does not.
func found(t *testing.T, l *Locations, slug string) (ingress.Heard, error) {
	t.Helper()

	return l.BySlug(context.Background(), "task", slug)
}

func TestLocations_Hear(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("what was heard of a resource is found by its uuid and by its slug", func(t *testing.T) {
		t.Parallel()

		l, c := on(t)
		heard := task("task-1", "web-abcde", "workload-orchestrator-01", c.now(), 80, 8080)

		l.Hear(ctx, heard)

		byUUID, err := l.ByUUID(ctx, "task", "task-1")
		require.NoError(t, err)
		assert.Equal(t, heard, byUUID)

		bySlug, err := l.BySlug(ctx, "task", "web-abcde")
		require.NoError(t, err)
		assert.Equal(t, heard, bySlug)
	})

	t.Run("what was never heard of is not there, and neither is what another kind was heard under", func(t *testing.T) {
		t.Parallel()

		l, c := on(t)
		l.Hear(ctx, task("task-1", "web-abcde", "workload-orchestrator-01", c.now()))

		for name, find := range map[string]func() (ingress.Heard, error){
			"a uuid nothing was heard under":      func() (ingress.Heard, error) { return l.ByUUID(ctx, "task", "task-9") },
			"a slug nothing was heard under":      func() (ingress.Heard, error) { return l.BySlug(ctx, "task", "web-zzzzz") },
			"a task's uuid, asked of the vms":     func() (ingress.Heard, error) { return l.ByUUID(ctx, "vm", "task-1") },
			"a task's slug, asked of the vms":     func() (ingress.Heard, error) { return l.BySlug(ctx, "vm", "web-abcde") },
			"no slug, which names nothing at all": func() (ingress.Heard, error) { return l.BySlug(ctx, "task", "") },
		} {
			_, err := find()
			assert.ErrorIs(t, err, domain.ErrNotExists, name)
		}
	})

	t.Run("one heard under no slug is found by its uuid alone, and one heard under one before keeps it", func(t *testing.T) {
		t.Parallel()

		l, c := on(t)
		l.Hear(ctx, task("task-1", "", "workload-orchestrator-01", c.now()))

		_, err := l.ByUUID(ctx, "task", "task-1")
		require.NoError(t, err)

		_, err = l.BySlug(ctx, "task", "")
		assert.ErrorIs(t, err, domain.ErrNotExists)

		l.Hear(ctx, task("task-2", "web-abcde", "workload-orchestrator-01", c.now()))
		l.Hear(ctx, doing(task("task-2", "", "workload-orchestrator-01", c.now().Add(time.Second)), stopped))

		heard, err := found(t, l, "web-abcde")
		require.NoError(t, err)
		assert.Equal(t, stopped, heard.State, "a slug is never taken from what it names")
	})

	t.Run("what has gone unheard for longer than the threshold is forgotten, and found until then", func(t *testing.T) {
		t.Parallel()

		l, c := on(t)
		l.Hear(ctx, task("task-1", "web-abcde", "workload-orchestrator-01", c.now()))

		c.pass(time.Minute)

		_, err := l.ByUUID(ctx, "task", "task-1")
		require.NoError(t, err, "a minute is not longer than a minute")

		c.pass(time.Second)

		_, err = l.ByUUID(ctx, "task", "task-1")
		assert.ErrorIs(t, err, domain.ErrNotExists)

		_, err = found(t, l, "web-abcde")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("one heard again is found again, for as long again", func(t *testing.T) {
		t.Parallel()

		l, c := on(t)
		l.Hear(ctx, task("task-1", "web-abcde", "workload-orchestrator-01", c.now()))

		c.pass(50 * time.Second)
		l.Hear(ctx, task("task-1", "web-abcde", "workload-orchestrator-01", c.now()))

		c.pass(50 * time.Second)

		_, err := found(t, l, "web-abcde")
		assert.NoError(t, err)
	})

	t.Run("and what is not found any more is not kept either", func(t *testing.T) {
		t.Parallel()

		l, c := on(t)
		l.Hear(ctx, task("long-gone", "old-abcde", "workload-orchestrator-01", c.now()))

		c.pass(2 * time.Minute)
		l.Hear(ctx, task("lately", "new-abcde", "workload-orchestrator-01", c.now()))

		assert.Len(t, l.kept["task"], 1, "what went unheard is let go of")
		assert.Contains(t, l.kept["task"], "lately")
		assert.Equal(t, map[string]string{"new-abcde": "lately"}, l.slugs["task"], "and so is the slug it was heard under")
	})

	t.Run("of two nodes that speak for one resource, the one heard last says where it is", func(t *testing.T) {
		t.Parallel()

		l, c := on(t)
		l.Hear(ctx, task("task-1", "web-abcde", "workload-orchestrator-01", c.now()))
		l.Hear(ctx, task("task-1", "web-abcde", "workload-orchestrator-02", c.now().Add(-time.Second)))

		heard, err := l.ByUUID(ctx, "task", "task-1")
		require.NoError(t, err)
		assert.Equal(t, "workload-orchestrator-02", heard.Node, "each node's word is held to its own clock alone")

		l.Hear(ctx, task("task-1", "web-abcde", "workload-orchestrator-01", c.now().Add(time.Second)))

		heard, err = found(t, l, "web-abcde")
		require.NoError(t, err)
		assert.Equal(t, "workload-orchestrator-01", heard.Node)
	})

	t.Run("what a node said before what was last taken from it is not taken", func(t *testing.T) {
		t.Parallel()

		l, c := on(t)
		l.Hear(ctx, doing(task("task-1", "web-abcde", "workload-orchestrator-01", c.now().Add(time.Second)), stopped))
		l.Hear(ctx, task("task-1", "web-abcde", "workload-orchestrator-01", c.now()))

		heard, err := found(t, l, "web-abcde")
		require.NoError(t, err)
		assert.Equal(t, stopped, heard.State, "a heartbeat taken before the one kept, and heard after it, says nothing new")
	})

	t.Run("a slug names the resource heard under it last, and one heard under another no longer names it", func(t *testing.T) {
		t.Parallel()

		l, c := on(t)
		l.Hear(ctx, task("task-1", "web-abcde", "workload-orchestrator-01", c.now()))
		l.Hear(ctx, task("task-1", "web-fghij", "workload-orchestrator-01", c.now().Add(time.Second)))

		_, err := found(t, l, "web-abcde")
		assert.ErrorIs(t, err, domain.ErrNotExists, "it is heard under another now")

		heard, err := found(t, l, "web-fghij")
		require.NoError(t, err)
		assert.Equal(t, "task-1", heard.UUID)

		l.Hear(ctx, task("task-2", "web-fghij", "workload-orchestrator-02", c.now().Add(2*time.Second)))
		l.Hear(ctx, task("task-1", "web-fghij", "workload-orchestrator-01", c.now().Add(time.Second)))

		heard, err = found(t, l, "web-fghij")
		require.NoError(t, err)
		assert.Equal(t, "task-2", heard.UUID, "heard under it later than the other")
	})

	t.Run("what is found shares nothing with what is kept", func(t *testing.T) {
		t.Parallel()

		l, c := on(t)

		ports := []port.Port{80}
		l.Hear(ctx, task("task-1", "web-abcde", "workload-orchestrator-01", c.now(), ports...))
		ports[0] = 22

		heard, err := l.ByUUID(ctx, "task", "task-1")
		require.NoError(t, err)
		heard.Ports[0] = 443

		again, err := l.ByUUID(ctx, "task", "task-1")
		require.NoError(t, err)
		assert.Equal(t, []port.Port{80}, again.Ports)
	})
}

func TestLocations_Withhold(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a command that leaves a resource out of being reached takes it away at once, and leaves what it lets in to tell", func(t *testing.T) {
		t.Parallel()

		l, c := on(t)
		l.Hear(ctx, task("task-1", "web-abcde", "workload-orchestrator-01", c.now(), 80, 8080))

		l.Withhold(ctx, ingress.Withheld{Kind: "task", UUID: "task-1", Command: "stop-1", State: stopped, Ports: []port.Port{80, 8080}})

		heard, err := found(t, l, "web-abcde")
		require.NoError(t, err)
		assert.Equal(t, stopped, heard.State)
		assert.Equal(t, []port.Port{80, 8080}, heard.Ports)
	})

	t.Run("one that leaves it reached on fewer ports narrows it to those both let in", func(t *testing.T) {
		t.Parallel()

		l, c := on(t)
		l.Hear(ctx, task("task-1", "web-abcde", "workload-orchestrator-01", c.now(), 80, 8080))

		l.Withhold(ctx, ingress.Withheld{Kind: "task", UUID: "task-1", Command: "reconfigure-1", Ports: []port.Port{8080, 9090}})

		heard, err := found(t, l, "web-abcde")
		require.NoError(t, err)
		assert.Equal(t, running, heard.State)
		assert.Equal(t, []port.Port{8080}, heard.Ports, "and never one it was not heard to let in")

		l.Withhold(ctx, ingress.Withheld{Kind: "task", UUID: "task-1", Command: "reconfigure-2"})

		heard, err = found(t, l, "web-abcde")
		require.NoError(t, err)
		assert.Empty(t, heard.Ports, "one that lets nothing in leaves nothing")
	})

	t.Run("a command never gives anything", func(t *testing.T) {
		t.Parallel()

		l, c := on(t)
		l.Hear(ctx, doing(task("task-1", "web-abcde", "workload-orchestrator-01", c.now(), 80), stopped))

		l.Withhold(ctx, ingress.Withheld{Kind: "task", UUID: "task-1", Command: "start-1", Ports: []port.Port{80, 8080}})

		heard, err := found(t, l, "web-abcde")
		require.NoError(t, err)
		assert.Equal(t, stopped, heard.State, "a start does not make it reached")
		assert.Equal(t, []port.Port{80}, heard.Ports)

		l.Withhold(ctx, ingress.Withheld{Kind: "task", UUID: "task-9", Command: "start-9", Ports: []port.Port{80}})

		_, err = l.ByUUID(ctx, "task", "task-9")
		assert.ErrorIs(t, err, domain.ErrNotExists, "nor does it make a route to what was never heard of")
	})

	t.Run("what it took away comes back with nothing its node says until it is answered", func(t *testing.T) {
		t.Parallel()

		l, c := on(t)
		l.Hear(ctx, task("task-1", "web-abcde", "workload-orchestrator-01", c.now(), 80))

		l.Withhold(ctx, ingress.Withheld{Kind: "task", UUID: "task-1", Command: "stop-1", State: stopping, Ports: []port.Port{80}})

		// its node beats before it has carried the stop out.
		l.Hear(ctx, task("task-1", "web-abcde", "workload-orchestrator-01", c.now().Add(time.Second), 80))

		heard, err := found(t, l, "web-abcde")
		require.NoError(t, err)
		assert.Equal(t, stopping, heard.State, "the heartbeat does not give back what the stop took away")

		// nor does what came of a command sent before it.
		late := task("task-1", "web-abcde", "workload-orchestrator-01", c.now().Add(time.Second), 80)
		l.Answer(ctx, ingress.Answered{Kind: "task", UUID: "task-1", Command: "start-0", Heard: &late})

		heard, err = found(t, l, "web-abcde")
		require.NoError(t, err)
		assert.Equal(t, stopping, heard.State)
	})

	t.Run("one that takes a resource nothing was heard of out of being reached gives it no route until it is answered", func(t *testing.T) {
		t.Parallel()

		l, c := on(t)
		l.Withhold(ctx, ingress.Withheld{Kind: "task", UUID: "task-1", Command: "delete-1", State: kind.Deleted})

		l.Hear(ctx, task("task-1", "web-abcde", "workload-orchestrator-01", c.now(), 80))

		_, err := found(t, l, "web-abcde")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("one that goes unanswered for as long as a resource may go unheard withholds nothing any more", func(t *testing.T) {
		t.Parallel()

		l, c := on(t)
		l.Hear(ctx, task("task-1", "web-abcde", "workload-orchestrator-01", c.now(), 80))
		l.Withhold(ctx, ingress.Withheld{Kind: "task", UUID: "task-1", Command: "stop-1", State: stopping})

		c.pass(time.Minute)
		l.Hear(ctx, task("task-1", "web-abcde", "workload-orchestrator-01", c.now(), 80))

		heard, err := found(t, l, "web-abcde")
		require.NoError(t, err)
		assert.Equal(t, stopping, heard.State, "a minute is not longer than a minute")

		c.pass(time.Second)
		l.Hear(ctx, task("task-1", "web-abcde", "workload-orchestrator-01", c.now(), 80))

		heard, err = found(t, l, "web-abcde")
		require.NoError(t, err)
		assert.Equal(t, running, heard.State, "its node is heard again")
	})
}

func TestLocations_Answer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("what came of a command is taken at once, and gives back what it withheld", func(t *testing.T) {
		t.Parallel()

		l, c := on(t)
		l.Hear(ctx, doing(task("task-1", "web-abcde", "workload-orchestrator-01", c.now()), stopped))

		started := task("task-1", "web-abcde", "workload-orchestrator-01", c.now().Add(time.Second), 80)
		l.Answer(ctx, ingress.Answered{Kind: "task", UUID: "task-1", Command: "start-1", Heard: &started})

		heard, err := found(t, l, "web-abcde")
		require.NoError(t, err)
		assert.Equal(t, running, heard.State)
		assert.Equal(t, []port.Port{80}, heard.Ports)

		l.Withhold(ctx, ingress.Withheld{Kind: "task", UUID: "task-1", Command: "stop-1", State: stopping, Ports: []port.Port{80}})

		stoppedNow := doing(task("task-1", "web-abcde", "workload-orchestrator-01", c.now().Add(2*time.Second), 80), stopped)
		l.Answer(ctx, ingress.Answered{Kind: "task", UUID: "task-1", Command: "stop-1", Heard: &stoppedNow})

		l.Hear(ctx, doing(task("task-1", "web-abcde", "workload-orchestrator-01", c.now().Add(3*time.Second), 80), stopped))

		heard, err = found(t, l, "web-abcde")
		require.NoError(t, err)
		assert.Equal(t, stopped, heard.State)
		assert.Equal(t, c.now().Add(3*time.Second), heard.At, "its node is heard again")
	})

	t.Run("one that says nothing changes nothing, but that its command is answered", func(t *testing.T) {
		t.Parallel()

		l, c := on(t)
		l.Hear(ctx, task("task-1", "web-abcde", "workload-orchestrator-01", c.now(), 80))
		l.Withhold(ctx, ingress.Withheld{Kind: "task", UUID: "task-1", Command: "stop-1", State: stopping, Ports: []port.Port{80}})

		l.Answer(ctx, ingress.Answered{Kind: "task", UUID: "task-1", Command: "stop-1"})

		heard, err := found(t, l, "web-abcde")
		require.NoError(t, err)
		assert.Equal(t, stopping, heard.State, "what failed gives nothing back of itself")

		l.Hear(ctx, task("task-1", "web-abcde", "workload-orchestrator-01", c.now().Add(time.Second), 80))

		heard, err = found(t, l, "web-abcde")
		require.NoError(t, err)
		assert.Equal(t, running, heard.State, "and its node is heard again")
	})

	t.Run("a resource a command deleted is gone, and nothing its node said before brings it back", func(t *testing.T) {
		t.Parallel()

		l, c := on(t)
		l.Hear(ctx, task("task-1", "web-abcde", "workload-orchestrator-01", c.now(), 80))

		gone := ingress.Heard{Kind: "task", UUID: "task-1", Node: "workload-orchestrator-01", At: c.now().Add(2 * time.Second), Gone: true}
		l.Answer(ctx, ingress.Answered{Kind: "task", UUID: "task-1", Command: "delete-1", Heard: &gone})

		l.Hear(ctx, task("task-1", "web-abcde", "workload-orchestrator-01", c.now().Add(time.Second), 80))

		_, err := l.ByUUID(ctx, "task", "task-1")
		assert.ErrorIs(t, err, domain.ErrNotExists)

		_, err = found(t, l, "web-abcde")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("and a heartbeat its node took before what came of a command never replaces it", func(t *testing.T) {
		t.Parallel()

		l, c := on(t)
		l.Hear(ctx, task("task-1", "web-abcde", "workload-orchestrator-01", c.now(), 80))

		stoppedNow := doing(task("task-1", "web-abcde", "workload-orchestrator-01", c.now().Add(2*time.Second), 80), stopped)
		l.Answer(ctx, ingress.Answered{Kind: "task", UUID: "task-1", Command: "stop-1", Heard: &stoppedNow})

		l.Hear(ctx, task("task-1", "web-abcde", "workload-orchestrator-01", c.now().Add(time.Second), 80))

		heard, err := found(t, l, "web-abcde")
		require.NoError(t, err)
		assert.Equal(t, stopped, heard.State)
	})
}

func TestLocations_concurrently(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	l := NewLocations(time.Minute)

	var everyone sync.WaitGroup

	for node := range 4 {
		everyone.Go(func() {
			for i := range 100 {
				uuid := fmt.Sprintf("task-%d", i%10)
				l.Hear(ctx, task(uuid, "web-"+uuid, fmt.Sprintf("workload-orchestrator-0%d", node), time.Now(), 80))
			}
		})

		everyone.Go(func() {
			for i := range 100 {
				uuid := fmt.Sprintf("task-%d", i%10)
				command := fmt.Sprintf("start-%d-%d", node, i)

				l.Withhold(ctx, ingress.Withheld{Kind: "task", UUID: uuid, Command: command, Ports: []port.Port{80}})

				answered := task(uuid, "web-"+uuid, fmt.Sprintf("workload-orchestrator-0%d", node), time.Now(), 80)
				l.Answer(ctx, ingress.Answered{Kind: "task", UUID: uuid, Command: command, Heard: &answered})
			}
		})

		everyone.Go(func() {
			for i := range 100 {
				uuid := fmt.Sprintf("task-%d", i%10)

				if heard, err := l.BySlug(ctx, "task", "web-"+uuid); err == nil {
					heard.Ports[0] = 22
				}

				_, _ = l.ByUUID(ctx, "task", uuid)
			}
		})
	}

	everyone.Wait()

	for i := range 10 {
		heard, err := l.BySlug(ctx, "task", fmt.Sprintf("web-task-%d", i))
		require.NoError(t, err)
		assert.Equal(t, []port.Port{80}, heard.Ports)
	}
}
