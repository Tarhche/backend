package gateway

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReplyRouting covers the guarantee the per-connection registry exists for:
// a session can only ever resolve a reply to a request it registered itself, so
// the ids one client chooses are its own and cannot address another's request.
func TestReplyRouting(t *testing.T) {
	t.Parallel()

	t.Run("a reply reaches only the client that asked for it", func(t *testing.T) {
		t.Parallel()

		g, requests := testGateway(t)

		clientA, servedA := serve(g, domain.Request{ID: "1", Subject: testSubject, Payload: []byte(`{"who":"a"}`)})
		clientB, servedB := serve(g, domain.Request{ID: "1", Subject: testSubject, Payload: []byte(`{"who":"b"}`)})

		requests.await(t, 2)

		require.NoError(t, g.Reply(context.Background(), &domain.Reply{
			RequestID: requests.get("a"),
			Payload:   []byte(`{"for":"a"}`),
		}))

		require.Eventually(t, func() bool { return len(clientA.written()) == 1 }, 2*time.Second, 5*time.Millisecond)

		assert.JSONEq(t, `{"for":"a"}`, string(clientA.written()[0].Payload))
		assert.Empty(t, clientB.written(), "a reply was delivered to a client that never asked for it")

		_ = clientA.Close()
		_ = clientB.Close()
		<-servedA
		<-servedB
	})

	t.Run("every connection is given its own registry", func(t *testing.T) {
		t.Parallel()

		g, requests := testGateway(t)

		clientA, servedA := serve(g, domain.Request{ID: "1", Subject: testSubject, Payload: []byte(`{"who":"a"}`)})
		clientB, servedB := serve(g, domain.Request{ID: "1", Subject: testSubject, Payload: []byte(`{"who":"b"}`)})

		requests.await(t, 2)

		// b's session must not be able to resolve a's request at all: with one
		// registry for the replica it could, and would answer in a's place.
		serverSideIDs := map[string]string{}

		g.hub.lock.RLock()
		for s := range g.hub.sessions {
			for _, who := range []string{"a", "b"} {
				if clientSideID, err := s.registry.GetClientSideID(requests.get(who)); err == nil {
					serverSideIDs[who] += clientSideID
				}
			}
		}
		g.hub.lock.RUnlock()

		// each request resolves in exactly one session, so each id is seen once
		assert.Equal(t, map[string]string{"a": "1", "b": "1"}, serverSideIDs)

		_ = clientA.Close()
		_ = clientB.Close()
		<-servedA
		<-servedB
	})

	t.Run("many clients may choose the same request id at the same time", func(t *testing.T) {
		t.Parallel()

		g, requests := testGateway(t)

		const clients = 25

		conns := make([]*fakeConn, 0, clients)
		served := make([]chan struct{}, 0, clients)
		names := make([]string, 0, clients)

		for i := range clients {
			who := fmt.Sprintf("c%d", i)

			// every client deliberately picks the same client-side id
			c, done := serve(g, domain.Request{
				ID:      "1",
				Subject: testSubject,
				Payload: fmt.Appendf(nil, `{"who":%q}`, who),
			})

			conns = append(conns, c)
			served = append(served, done)
			names = append(names, who)
		}

		requests.await(t, clients)

		var group sync.WaitGroup
		for _, who := range names {

			group.Go(func() {

				assert.NoError(t, g.Reply(context.Background(), &domain.Reply{
					RequestID: requests.get(who),
					Payload:   fmt.Appendf(nil, `{"for":%q}`, who),
				}))
			})
		}
		group.Wait()

		for i, c := range conns {
			require.Eventually(t, func() bool { return len(c.written()) == 1 }, 5*time.Second, 5*time.Millisecond,
				"client %d was never answered", i)

			written := c.written()[0]

			assert.Equal(t, "1", written.RequestID)
			assert.JSONEq(t, fmt.Sprintf(`{"for":%q}`, names[i]), string(written.Payload),
				"client %d got another client's reply", i)
		}

		for i, c := range conns {
			_ = c.Close()
			<-served[i]
		}
	})

	t.Run("server side ids are unique across connections", func(t *testing.T) {
		t.Parallel()

		g, requests := testGateway(t)

		const clients = 10

		conns := make([]*fakeConn, 0, clients)
		served := make([]chan struct{}, 0, clients)

		for i := range clients {
			c, done := serve(g, domain.Request{
				ID:      "1",
				Subject: testSubject,
				Payload: fmt.Appendf(nil, `{"who":"c%d"}`, i),
			})

			conns = append(conns, c)
			served = append(served, done)
		}

		requests.await(t, clients)

		seen := make(map[string]struct{}, clients)
		for i := range clients {
			id := requests.get(fmt.Sprintf("c%d", i))

			require.NotEmpty(t, id)
			assert.NotContains(t, seen, id, "two connections were given the same server-side id")

			seen[id] = struct{}{}
		}

		for i, c := range conns {
			_ = c.Close()
			<-served[i]
		}
	})
}
