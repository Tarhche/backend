package graphtest_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/graph/graphtest"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

var (
	vm  = graph.ID("vm", "v-uuid")
	web = graph.ID("container", "web")
)

func TestKind(t *testing.T) {
	t.Parallel()

	t.Run("a node requires what it is said to, and nothing else", func(t *testing.T) {
		t.Parallel()

		kinds := graphtest.NewKind()
		kinds.Require(web, graphtest.Requirement("vm", "v-uuid"))

		required, err := kinds.Requires(web)
		require.NoError(t, err)
		assert.Equal(t, []graph.Requirement{{Kind: "vm", Name: "v-uuid", Spec: vm}}, required, "its spec its id")

		required, err = kinds.Requires(vm)
		require.NoError(t, err)
		assert.Empty(t, required, "and one it is told nothing of requires nothing")
	})

	t.Run("a spec that is no node's id is not read", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		kinds := graphtest.NewKind()

		_, err := kinds.Requires("web")
		assert.ErrorIs(t, err, kind.ErrInvalidPayload)

		_, _, err = kinds.Exists(ctx, "web")
		assert.ErrorIs(t, err, kind.ErrInvalidPayload)

		_, err = kinds.Create(ctx, "web", nil)
		assert.ErrorIs(t, err, kind.ErrInvalidPayload)

		assert.Empty(t, kinds.Calls(), "and is no node asked anything")
	})

	t.Run("a node is there once it is made, and not once it is removed", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		kinds := graphtest.NewKind()

		_, there, err := kinds.Exists(ctx, web)
		require.NoError(t, err)
		assert.False(t, there)

		ref, err := kinds.Create(ctx, web, map[graph.NodeID]graph.Ref{vm: graphtest.Ref(vm)})
		require.NoError(t, err)
		assert.Equal(t, graph.Ref{Kind: "container", UUID: "web"}, ref, "its record its kind and its name")

		found, there, err := kinds.Exists(ctx, web)
		require.NoError(t, err)
		assert.True(t, there)
		assert.Equal(t, ref, found)
		assert.Equal(t, []graph.NodeID{web}, kinds.Held())

		require.NoError(t, kinds.Delete(ctx, ref))

		_, there, err = kinds.Exists(ctx, web)
		require.NoError(t, err)
		assert.False(t, there)
		assert.Empty(t, kinds.Held())

		assert.NoError(t, kinds.Delete(ctx, ref), "and one gone already is removed")
	})

	t.Run("a root's record is the root of the node of its kind named by its uuid", func(t *testing.T) {
		t.Parallel()

		kinds := graphtest.NewKind()

		root, err := kinds.Root(kind.Raw{Kind: "container", Metadata: kind.Metadata{UUID: "web", Name: "anything"}})
		require.NoError(t, err)
		assert.Equal(t, graphtest.Requirement("container", "web"), root)
		assert.Equal(t, web, root.ID())

		assert.Equal(t, []graphtest.Call{{Op: graphtest.Root, Node: web}}, kinds.Calls(), "and it is recorded as asked")

		broken := errors.New("broken")
		kinds.On(graphtest.Root, web, func(context.Context) error { return broken })

		_, err = kinds.Root(kind.Raw{Kind: "container", Metadata: kind.Metadata{UUID: "web"}})
		assert.ErrorIs(t, err, broken, "doing first what a test has it do")

		for name, r := range map[string]kind.Raw{
			"of no uuid": {Kind: "container"},
			"of no kind": {Metadata: kind.Metadata{UUID: "web"}},
		} {
			_, err := kinds.Root(r)
			assert.ErrorIs(t, err, kind.ErrInvalidPayload, "a record %s is no root", name)
		}
	})

	t.Run("a record not made is its record all the same, and not made, until it is made", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		kinds := graphtest.NewKind()
		kinds.Record(web)

		ref, made, err := kinds.Exists(ctx, web)
		require.NoError(t, err)
		assert.False(t, made)
		assert.Equal(t, graphtest.Ref(web), ref, "its record")
		assert.Equal(t, []graph.NodeID{web}, kinds.Held(), "which it holds")

		ref, made, err = kinds.Exists(ctx, vm)
		require.NoError(t, err)
		assert.False(t, made)
		assert.Zero(t, ref, "and one it holds no record of is none")

		_, err = kinds.Create(ctx, web, nil)
		require.NoError(t, err)

		ref, made, err = kinds.Exists(ctx, web)
		require.NoError(t, err)
		assert.True(t, made, "once it is made")
		assert.Equal(t, graphtest.Ref(web), ref)

		kinds.Record(web)

		_, made, err = kinds.Exists(ctx, web)
		require.NoError(t, err)
		assert.False(t, made, "and not once a test says it is not")

		require.NoError(t, kinds.Delete(ctx, ref))
		assert.Empty(t, kinds.Held(), "and it goes when it is removed, made or not")
	})

	t.Run("a record a delete is told to leave is left, not made, until it is forgotten", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		kinds := graphtest.NewKind()
		kinds.Hold(web, vm)
		kinds.Leave(web)

		require.NoError(t, kinds.Delete(ctx, graphtest.Ref(web)))
		require.NoError(t, kinds.Delete(ctx, graphtest.Ref(vm)))

		ref, made, err := kinds.Exists(ctx, web)
		require.NoError(t, err)
		assert.False(t, made)
		assert.Equal(t, graphtest.Ref(web), ref, "its record is there")
		assert.Equal(t, []graph.NodeID{web}, kinds.Held(), "while one it is not told to leave goes")

		require.NoError(t, kinds.Ownership().Forget(ctx, graphtest.Ref(web)))
		assert.Empty(t, kinds.Held(), "until it is forgotten")

		require.NoError(t, kinds.Delete(ctx, graphtest.Ref(web)))
		assert.Empty(t, kinds.Held(), "and one gone is not left")
	})

	t.Run("a node held is there as if it were made before", func(t *testing.T) {
		t.Parallel()

		kinds := graphtest.NewKind()
		kinds.Hold(vm, web)

		ref, there, err := kinds.Exists(context.Background(), vm)
		require.NoError(t, err)
		assert.True(t, there)
		assert.Equal(t, graphtest.Ref(vm), ref)

		assert.Equal(t, []graph.NodeID{web, vm}, kinds.Held(), "in the order of their ids")
		assert.Empty(t, kinds.Asked(graphtest.Create))
	})

	t.Run("what a node is told to do it does first, and fails with what that fails with", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		broken := errors.New("broken")

		kinds := graphtest.NewKind()
		kinds.On(graphtest.Create, web, func(context.Context) error { return broken })

		_, err := kinds.Create(ctx, web, nil)
		assert.ErrorIs(t, err, broken)
		assert.Empty(t, kinds.Held(), "and is not made")

		_, err = kinds.Create(ctx, vm, nil)
		require.NoError(t, err, "while another node is")

		kinds.On(graphtest.Create, web, nil)

		_, err = kinds.Create(ctx, web, nil)
		require.NoError(t, err, "and so is the node once it is told nothing")
		assert.Equal(t, []graph.NodeID{web, vm}, kinds.Held())

		kinds.On(graphtest.Delete, web, func(ctx context.Context) error {
			<-ctx.Done()

			return ctx.Err()
		})

		cancelled, cancel := context.WithCancel(ctx)
		cancel()

		assert.ErrorIs(t, kinds.Delete(cancelled, graphtest.Ref(web)), context.Canceled, "with the context it is asked with")
		assert.Equal(t, []graph.NodeID{web, vm}, kinds.Held(), "and is not removed")

		kinds.On(graphtest.Exists, vm, func(context.Context) error { return &graph.Waiting{Reason: "its vm is stopped"} })

		_, _, err = kinds.Exists(ctx, vm)

		var waiting *graph.Waiting
		require.ErrorAs(t, err, &waiting, "and may wait")
		assert.Equal(t, "its vm is stopped", waiting.Reason)

		kinds.Require(web, graphtest.Requirement("vm", "v-uuid"))
		kinds.On(graphtest.Requires, web, func(context.Context) error { return broken })

		_, err = kinds.Requires(web)
		assert.ErrorIs(t, err, broken, "whatever it is asked")
	})

	t.Run("everything it is asked is recorded, in order", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		kinds := graphtest.NewKind()

		deps := map[graph.NodeID]graph.Ref{vm: graphtest.Ref(vm)}

		_, err := kinds.Requires(web)
		require.NoError(t, err)
		_, _, err = kinds.Exists(ctx, web)
		require.NoError(t, err)
		_, err = kinds.Create(ctx, web, deps)
		require.NoError(t, err)
		require.NoError(t, kinds.Delete(ctx, graphtest.Ref(web)))

		deps[vm] = graph.Ref{}

		calls := kinds.Calls()
		assert.Equal(t, []graphtest.Call{
			{Op: graphtest.Requires, Node: web},
			{Op: graphtest.Exists, Node: web},
			{Op: graphtest.Create, Node: web, Deps: map[graph.NodeID]graph.Ref{vm: {Kind: "vm", UUID: "v-uuid"}}},
			{Op: graphtest.Delete, Node: web},
		}, calls, "a create with what it was handed then")

		calls[2].Deps[vm] = graph.Ref{}
		assert.Equal(t, graphtest.Ref(vm), kinds.Calls()[2].Deps[vm], "and what it answers is a copy of its own")

		assert.Equal(t, []graph.NodeID{web}, kinds.Asked(graphtest.Create))
		assert.Empty(t, kinds.Asked("restart"))
	})

	t.Run("when each call began and when it returned is recorded, in the order they did", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		broken := errors.New("broken")

		kinds := graphtest.NewKind()
		kinds.On(graphtest.Requires, vm, func(context.Context) error { return broken })

		// web is made while vm is looked for, and returns once it has been.
		began := make(chan struct{})
		looked := make(chan struct{})
		kinds.On(graphtest.Create, web, func(context.Context) error {
			close(began)
			<-looked

			return nil
		})

		var making sync.WaitGroup
		making.Go(func() {
			_, err := kinds.Create(ctx, web, nil)
			assert.NoError(t, err)
		})

		<-began

		_, _, err := kinds.Exists(ctx, vm)
		require.NoError(t, err)

		close(looked)
		making.Wait()

		_, err = kinds.Requires(vm)
		require.ErrorIs(t, err, broken)

		_, err = kinds.Requires("vm")
		require.ErrorIs(t, err, kind.ErrInvalidPayload)

		assert.Equal(t, []graphtest.Event{
			{Op: graphtest.Create, Node: web},
			{Op: graphtest.Exists, Node: vm},
			{Op: graphtest.Exists, Node: vm, Returned: true},
			{Op: graphtest.Create, Node: web, Returned: true},
			{Op: graphtest.Requires, Node: vm},
			{Op: graphtest.Requires, Node: vm, Returned: true},
		}, kinds.Events(), "one that failed returned too, and a spec that is no node's id is no call")
	})

	t.Run("one kind may be every kind of a graph", func(t *testing.T) {
		t.Parallel()

		kinds := graphtest.NewKind()

		assert.Equal(t, graph.Kinds{"vm": kinds, "container": kinds}, kinds.Kinds("vm", "container"))
	})

	t.Run("it may be asked from several goroutines at once, each waiting for the others", func(t *testing.T) {
		t.Parallel()

		const nodes = 20

		ctx := context.Background()
		kinds := graphtest.NewKind()

		// no create is over before every one has started.
		var started sync.WaitGroup
		started.Add(nodes)

		var wg sync.WaitGroup
		for i := range nodes {
			id := graph.ID("container", fmt.Sprintf("c-%d", i))
			kinds.On(graphtest.Create, id, func(context.Context) error {
				started.Done()
				started.Wait()

				return nil
			})

			wg.Go(func() {
				_, err := kinds.Create(ctx, id, map[graph.NodeID]graph.Ref{vm: graphtest.Ref(vm)})
				assert.NoError(t, err)

				_, there, err := kinds.Exists(ctx, id)
				assert.NoError(t, err)
				assert.True(t, there)

				_ = kinds.Calls()
				_ = kinds.Held()
			})
		}

		wg.Wait()

		assert.Len(t, kinds.Held(), nodes)
		assert.Len(t, kinds.Asked(graphtest.Create), nodes)
		assert.Len(t, kinds.Asked(graphtest.Exists), nodes)
	})
}

func TestOwnership(t *testing.T) {
	t.Parallel()

	network := graph.ID("network", "s_default")
	api := graph.ID("container", "api")
	stack := graph.ID("stack", "s-uuid")

	ref := graphtest.Ref

	t.Run("what owns a record is what a test says, and each owns it while its record is there", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		kinds := graphtest.NewKind()
		ownership := kinds.Ownership()

		kinds.Hold(network, web, stack)
		ownership.Own(network, web, stack)
		ownership.Own(network, api, web)

		assert.Equal(t, []graph.NodeID{api, web, stack}, ownership.OwnersOf(network), "each once, in the order of their ids")

		owners, err := ownership.Owners(ctx, ref(network))
		require.NoError(t, err)
		assert.Equal(t, []graph.Ref{ref(web), ref(stack)}, owners, "api, which it holds no record of, owns nothing")

		kinds.Record(api)

		owners, err = ownership.Owners(ctx, ref(network))
		require.NoError(t, err)
		assert.Equal(t, []graph.Ref{ref(api), ref(web), ref(stack)}, owners, "until it holds one, made or not, in the order of their kinds and uuids")

		require.NoError(t, kinds.Delete(ctx, ref(web)))

		owners, err = ownership.Owners(ctx, ref(network))
		require.NoError(t, err)
		assert.Equal(t, []graph.Ref{ref(api), ref(stack)}, owners, "and one whose record is gone owns nothing")
		assert.Equal(t, []graph.NodeID{api, web, stack}, ownership.OwnersOf(network), "though the record names it still")
	})

	t.Run("disowning takes owners out of what owns a record, and is what owns it still", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		kinds := graphtest.NewKind()
		ownership := kinds.Ownership()

		kinds.Hold(network, web, api, stack)
		ownership.Own(network, web, api, stack)

		owners, err := ownership.Disown(ctx, ref(network), ref(web), ref(stack), ref(vm))
		require.NoError(t, err)
		assert.Equal(t, []graph.Ref{ref(api)}, owners, "what does not own it is taken out of nothing")
		assert.Equal(t, []graph.NodeID{api}, ownership.OwnersOf(network))

		owners, err = ownership.Disown(ctx, ref(network), ref(api))
		require.NoError(t, err)
		assert.Empty(t, owners)
		assert.ElementsMatch(t, []graph.NodeID{network, web, api, stack}, kinds.Held(), "and no record goes for it")
	})

	t.Run("a record that is gone is owned by nothing, disowned already, and forgotten already", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		kinds := graphtest.NewKind()
		ownership := kinds.Ownership()

		kinds.Hold(web)
		ownership.Own(network, web)

		owners, err := ownership.Owners(ctx, ref(network))
		require.NoError(t, err)
		assert.Empty(t, owners)

		owners, err = ownership.Disown(ctx, ref(network), ref(web))
		require.NoError(t, err)
		assert.Empty(t, owners)

		assert.NoError(t, ownership.Forget(ctx, ref(stack)))
	})

	t.Run("what owns a record goes with it, whether it is removed or forgotten", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		kinds := graphtest.NewKind()
		ownership := kinds.Ownership()

		kinds.Hold(network, web, stack)
		ownership.Own(network, web)
		ownership.Own(web, stack)

		require.NoError(t, kinds.Delete(ctx, ref(network)))
		assert.Empty(t, ownership.OwnersOf(network))

		require.NoError(t, ownership.Forget(ctx, ref(web)))
		assert.Empty(t, ownership.OwnersOf(web))
		assert.Equal(t, []graph.NodeID{stack}, kinds.Held(), "and forgetting a record lets it go")
	})

	t.Run("what it is asked is recorded among what the kind is, and does first what a test has it do", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		broken := errors.New("broken")

		kinds := graphtest.NewKind()
		ownership := kinds.Ownership()

		kinds.Hold(network, web, stack)
		ownership.Own(network, web, stack)

		kinds.On(graphtest.Disown, network, func(context.Context) error { return broken })

		_, err := ownership.Disown(ctx, ref(network), ref(web))
		require.ErrorIs(t, err, broken)
		assert.Equal(t, []graph.NodeID{web, stack}, ownership.OwnersOf(network), "and is disowned of nothing")

		kinds.On(graphtest.Owners, network, func(context.Context) error { return broken })

		_, err = ownership.Owners(ctx, ref(network))
		require.ErrorIs(t, err, broken)

		kinds.On(graphtest.Forget, stack, func(ctx context.Context) error {
			<-ctx.Done()

			return ctx.Err()
		})

		cancelled, cancel := context.WithCancel(ctx)
		cancel()

		require.ErrorIs(t, ownership.Forget(cancelled, ref(stack)), context.Canceled, "with the context it is asked with")
		assert.Contains(t, kinds.Held(), stack, "and is not forgotten")

		_, _, err = kinds.Exists(ctx, network)
		require.NoError(t, err)

		assert.Equal(t, []graphtest.Call{
			{Op: graphtest.Disown, Node: network, Owners: []graph.Ref{ref(web)}},
			{Op: graphtest.Owners, Node: network},
			{Op: graphtest.Forget, Node: stack},
			{Op: graphtest.Exists, Node: network},
		}, kinds.Calls(), "a disown with the owners it was handed")

		assert.Equal(t, []graphtest.Event{
			{Op: graphtest.Disown, Node: network},
			{Op: graphtest.Disown, Node: network, Returned: true},
			{Op: graphtest.Owners, Node: network},
			{Op: graphtest.Owners, Node: network, Returned: true},
			{Op: graphtest.Forget, Node: stack},
			{Op: graphtest.Forget, Node: stack, Returned: true},
			{Op: graphtest.Exists, Node: network},
			{Op: graphtest.Exists, Node: network, Returned: true},
		}, kinds.Events())

		assert.Equal(t, []graph.NodeID{network}, kinds.Asked(graphtest.Disown))
	})

	t.Run("linking an owned edge has the dependent own what it needs, and not use it, but for what it lives in", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		kinds := graphtest.NewKind()
		ownership := kinds.Ownership()

		kinds.Hold(vm, network, web)
		ownership.Parent("container", "vm")
		ownership.Use(web, vm, network)

		require.NoError(t, ownership.Link(ctx, ref(network), ref(web), graph.Owned))
		assert.Equal(t, []graph.NodeID{web}, ownership.OwnersOf(network))
		assert.Equal(t, []graph.NodeID{vm}, ownership.RefsOf(web), "what it owns it does not use")

		require.NoError(t, ownership.Link(ctx, ref(vm), ref(web), graph.Owned))
		assert.Equal(t, []graph.NodeID{web}, ownership.OwnersOf(vm))
		assert.Equal(t, []graph.NodeID{vm}, ownership.RefsOf(web), "and what it lives in it uses whatever owns what")

		owners, err := ownership.Owners(ctx, ref(vm))
		require.NoError(t, err)
		assert.Equal(t, []graph.Ref{ref(web)}, owners, "a vm lives in nothing, and what owns it owns it")
	})

	t.Run("linking a loose edge has the dependent use what it needs, and not own it", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		kinds := graphtest.NewKind()
		ownership := kinds.Ownership()

		kinds.Hold(network, web, api)
		ownership.Own(network, web, api)

		require.NoError(t, ownership.Link(ctx, ref(network), ref(web), graph.Loose))
		assert.Equal(t, []graph.NodeID{network}, ownership.RefsOf(web))
		assert.Equal(t, []graph.NodeID{api}, ownership.OwnersOf(network), "and what else owns it still does")

		require.NoError(t, ownership.Link(ctx, ref(network), ref(web), graph.Loose))
		assert.Equal(t, []graph.NodeID{network}, ownership.RefsOf(web), "and linking it again changes nothing")
		assert.Equal(t, []graph.NodeID{api}, ownership.OwnersOf(network))

		require.NoError(t, ownership.Link(ctx, ref(network), ref(web), graph.Owned))
		assert.Empty(t, ownership.RefsOf(web), "until it is linked as owned")
		assert.Equal(t, []graph.NodeID{api, web}, ownership.OwnersOf(network))
	})

	t.Run("an order links nothing, and an edge of no type is refused", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		kinds := graphtest.NewKind()
		ownership := kinds.Ownership()

		kinds.Hold(web, api)

		require.NoError(t, ownership.Link(ctx, ref(api), ref(web), graph.Order))
		assert.Empty(t, ownership.RefsOf(web))
		assert.Empty(t, ownership.OwnersOf(api))

		assert.Error(t, ownership.Link(ctx, ref(api), ref(web), graph.EdgeType(0)))
		assert.Empty(t, ownership.RefsOf(web))
	})

	t.Run("a record that is gone is linked to nothing", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		kinds := graphtest.NewKind()
		ownership := kinds.Ownership()

		kinds.Hold(web)

		require.NoError(t, ownership.Link(ctx, ref(network), ref(web), graph.Loose))
		require.NoError(t, ownership.Link(ctx, ref(network), ref(web), graph.Owned))
		assert.Empty(t, ownership.OwnersOf(network), "the network, which it does not hold, is owned by nothing")

		require.NoError(t, ownership.Link(ctx, ref(web), ref(stack), graph.Loose))
		assert.Empty(t, ownership.RefsOf(stack), "and the stack, which it does not hold, uses nothing")
	})

	t.Run("what a record uses goes with it", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		kinds := graphtest.NewKind()
		ownership := kinds.Ownership()

		kinds.Hold(web, stack)
		ownership.Use(web, vm, vm, network)
		ownership.Use(stack, web)

		assert.Equal(t, []graph.NodeID{network, vm}, ownership.RefsOf(web), "each once, in the order of their ids")

		require.NoError(t, kinds.Delete(ctx, ref(web)))
		assert.Empty(t, ownership.RefsOf(web))

		require.NoError(t, ownership.Forget(ctx, ref(stack)))
		assert.Empty(t, ownership.RefsOf(stack))
	})

	t.Run("a link is recorded with what it links and how, and does first what a test has it do", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		broken := errors.New("broken")

		kinds := graphtest.NewKind()
		ownership := kinds.Ownership()

		kinds.Hold(network, web)
		kinds.On(graphtest.Link, web, func(context.Context) error { return broken })

		require.ErrorIs(t, ownership.Link(ctx, ref(network), ref(web), graph.Owned), broken)
		assert.Empty(t, ownership.OwnersOf(network), "and links nothing")

		kinds.On(graphtest.Link, web, nil)
		require.NoError(t, ownership.Link(ctx, ref(network), ref(web), graph.Loose))

		assert.Equal(t, []graphtest.Call{
			{Op: graphtest.Link, Node: web, Needed: ref(network), Type: graph.Owned},
			{Op: graphtest.Link, Node: web, Needed: ref(network), Type: graph.Loose},
		}, kinds.Calls())

		assert.Equal(t, []graphtest.Event{
			{Op: graphtest.Link, Node: web},
			{Op: graphtest.Link, Node: web, Returned: true},
			{Op: graphtest.Link, Node: web},
			{Op: graphtest.Link, Node: web, Returned: true},
		}, kinds.Events())
	})

	t.Run("it may be asked from several goroutines at once", func(t *testing.T) {
		t.Parallel()

		const owners = 20

		ctx := context.Background()
		kinds := graphtest.NewKind()
		ownership := kinds.Ownership()
		kinds.Hold(network)

		var wg sync.WaitGroup
		for i := range owners {
			owner := graph.ID("container", fmt.Sprintf("c-%d", i))

			wg.Go(func() {
				kinds.Hold(owner)
				ownership.Own(network, owner)

				_, err := ownership.Owners(ctx, ref(network))
				assert.NoError(t, err)

				_, err = ownership.Disown(ctx, ref(network), ref(owner))
				assert.NoError(t, err)

				assert.NoError(t, ownership.Link(ctx, ref(network), ref(owner), graph.Loose))

				_ = ownership.OwnersOf(network)
				_ = ownership.RefsOf(owner)
			})
		}

		wg.Wait()

		assert.Empty(t, ownership.OwnersOf(network), "each disowned of what it was owned by")
		assert.Len(t, kinds.Asked(graphtest.Disown), owners)
		assert.Len(t, kinds.Asked(graphtest.Link), owners)
	})
}
