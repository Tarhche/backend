package tunnel

import (
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseForward(t *testing.T) {
	t.Run("what a rule can say", func(t *testing.T) {
		tests := []struct {
			rule string
			want Forward
		}{
			{
				rule: "8022=agent-a:22",
				want: Forward{Address: ":8022", Agent: "agent-a", Target: Target{Host: "127.0.0.1", Port: 22}},
			},
			{
				rule: "8080=agent-a:api",
				want: Forward{Address: ":8080", Agent: "agent-a", Target: Target{Service: "api"}},
			},
			{
				rule: "5432=agent-b:10.0.0.5:5432",
				want: Forward{Address: ":5432", Agent: "agent-b", Target: Target{Host: "10.0.0.5", Port: 5432}},
			},
			{
				rule: "9000=:api",
				want: Forward{Address: ":9000", Target: Target{Service: "api"}},
			},
			{
				rule: "127.0.0.1:8022=agent-a:22",
				want: Forward{Address: "127.0.0.1:8022", Agent: "agent-a", Target: Target{Host: "127.0.0.1", Port: 22}},
			},
			{
				rule: "  8022 = agent-a : 22  ",
				want: Forward{Address: ":8022", Agent: "agent-a", Target: Target{Host: "127.0.0.1", Port: 22}},
			},
		}

		for _, test := range tests {
			t.Run(test.rule, func(t *testing.T) {
				forward, err := ParseForward(test.rule)
				require.NoError(t, err)
				assert.Equal(t, test.want, forward)
			})
		}
	})

	t.Run("what a rule cannot say", func(t *testing.T) {
		tests := []string{
			"8022",             // no destination
			"8022=agent-a",     // no target
			"=agent-a:22",      // nothing to listen on
			"ssh=agent-a:22",   // not a port
			"70000=agent-a:22", // not a port either
			"8022=agent-a:",    // no target
			"8022=agent-a:0",   // port zero reaches nothing
			"8022=:",           // neither agent nor target
		}

		for _, rule := range tests {
			t.Run(rule, func(t *testing.T) {
				_, err := ParseForward(rule)
				assert.ErrorIs(t, err, ErrMalformedForward)
			})
		}
	})

	t.Run("a list is read whole, and an empty one forwards nothing", func(t *testing.T) {
		forwards, err := ParseForwards("8022=agent-a:22, 9000=:api")
		require.NoError(t, err)
		require.Len(t, forwards, 2)
		assert.Equal(t, "agent-a", forwards[0].Agent)
		assert.Empty(t, forwards[1].Agent)

		forwards, err = ParseForwards("")
		require.NoError(t, err)
		assert.Empty(t, forwards)

		_, err = ParseForwards("8022=agent-a:22,nonsense")
		assert.ErrorIs(t, err, ErrMalformedForward)
	})
}

func TestParseAddressRules(t *testing.T) {
	t.Run("a port, and a span of them", func(t *testing.T) {
		rules, err := ParseAddressRules("127.0.0.1:5432, 127.0.0.1:30000-31000")
		require.NoError(t, err)
		require.Len(t, rules, 2)

		assert.Equal(t, AddressRule{Host: "127.0.0.1", Ports: []uint16{5432}}, rules[0])
		assert.Equal(t, AddressRule{Host: "127.0.0.1", From: 30000, To: 31000}, rules[1])

		assert.True(t, rules[1].allows("127.0.0.1", 30000))
		assert.True(t, rules[1].allows("127.0.0.1", 31000))
		assert.False(t, rules[1].allows("127.0.0.1", 31001))
		assert.False(t, rules[1].allows("10.0.0.1", 30500))
	})

	t.Run("what is not an address", func(t *testing.T) {
		for _, rule := range []string{"127.0.0.1", ":5432", "127.0.0.1:ssh", "127.0.0.1:0", "127.0.0.1:31000-30000"} {
			t.Run(rule, func(t *testing.T) {
				_, err := ParseAddressRules(rule)
				assert.ErrorIs(t, err, ErrTargetNotAllowed)
			})
		}
	})

	t.Run("nothing named allows nothing", func(t *testing.T) {
		rules, err := ParseAddressRules("")
		require.NoError(t, err)
		assert.Empty(t, rules)
	})
}

// startForwarder serves the given forwards until the test ends, and reports
// where each one actually landed.
func startForwarder(t *testing.T, hub *Hub, forwards ...Forward) []string {
	t.Helper()

	forwarder, err := NewForwarder(hub, discardLogger(), forwards...)
	require.NoError(t, err)
	require.NoError(t, forwarder.Listen())

	served := make(chan error, 1)
	go func() { served <- forwarder.Serve(t.Context()) }()

	t.Cleanup(func() {
		require.NoError(t, forwarder.Close())
		assert.NoError(t, <-served)
	})

	addresses := make([]string, 0, len(forwards))
	for _, address := range forwarder.Listening() {
		addresses = append(addresses, address.String())
	}

	return addresses
}

func anyPort() string { return "127.0.0.1:0" }

func TestForwardedPortCarriesArbitraryTCP(t *testing.T) {
	config := testConfig()
	hub := startHub(t, config, AllowAll())

	startAgent(t, "agent-a", []string{hub.address}, config,
		NewServiceTargets(map[string]string{"echo": echoServer(t, "agent-a:")}),
	)
	waitFor(t, "the agent", func() bool { return len(hub.Agents()) == 1 })

	ports := startForwarder(t, hub.Hub, Forward{
		Address: anyPort(),
		Agent:   "agent-a",
		Target:  Target{Service: "echo"},
	})

	// a plain TCP client, which knows nothing of any of this
	client, err := net.Dial("tcp", ports[0])
	require.NoError(t, err)
	defer client.Close()

	greeting := make([]byte, len("agent-a:"))
	require.NoError(t, client.SetDeadline(time.Now().Add(5*time.Second)))
	_, err = io.ReadFull(client, greeting)
	require.NoError(t, err)
	assert.Equal(t, "agent-a:", string(greeting))

	assert.Equal(t, "hello", roundTrip(t, client, "hello"))
}

func TestForwardedPortReachesAnAllowedAddress(t *testing.T) {
	config := testConfig()
	hub := startHub(t, config, AllowAll())

	target := echoServer(t, "")
	_, port, err := net.SplitHostPort(target)
	require.NoError(t, err)
	number, err := strconv.ParseUint(port, 10, 16)
	require.NoError(t, err)

	// the agent offers no services at all, only this one address
	startAgent(t, "agent-a", []string{hub.address}, config,
		NewServiceTargets(nil, AddressRule{Host: "127.0.0.1", Ports: []uint16{uint16(number)}}),
	)
	waitFor(t, "the agent", func() bool { return len(hub.Agents()) == 1 })

	ports := startForwarder(t, hub.Hub,
		Forward{Address: anyPort(), Agent: "agent-a", Target: Target{Host: "127.0.0.1", Port: uint16(number)}},
		Forward{Address: anyPort(), Agent: "agent-a", Target: Target{Host: "127.0.0.1", Port: uint16(number) + 1}},
	)

	client, err := net.Dial("tcp", ports[0])
	require.NoError(t, err)
	defer client.Close()

	assert.Equal(t, "hello", roundTrip(t, client, "hello"))

	// the port next door was never allowed, so the agent refuses and there is
	// nothing to say at layer four but to close
	refused, err := net.Dial("tcp", ports[1])
	require.NoError(t, err)
	defer refused.Close()

	require.NoError(t, refused.SetDeadline(time.Now().Add(5*time.Second)))
	_, err = refused.Read(make([]byte, 1))
	assert.ErrorIs(t, err, io.EOF)
}

func TestForwardedPortRoutesWhenItNamesNoAgent(t *testing.T) {
	config := testConfig()
	hub := startHub(t, config, AllowAll())

	for _, name := range []string{"agent-a", "agent-b"} {
		startAgent(t, name, []string{hub.address}, config,
			NewServiceTargets(map[string]string{"echo": echoServer(t, name+":")}),
		)
	}
	waitFor(t, "both agents", func() bool { return len(hub.Agents()) == 2 })

	ports := startForwarder(t, hub.Hub, Forward{
		Address: anyPort(),
		Target:  Target{Service: "echo"},
	})

	reached := make(map[string]bool)

	for range 6 {
		client, err := net.Dial("tcp", ports[0])
		require.NoError(t, err)

		greeting := make([]byte, len("agent-a:"))
		require.NoError(t, client.SetDeadline(time.Now().Add(5*time.Second)))
		_, err = io.ReadFull(client, greeting)
		require.NoError(t, err)

		reached[string(greeting)] = true
		client.Close()
	}

	assert.Len(t, reached, 2, "the least loaded agent should not always be the same one")
}

func TestForwardedPortWithNoAgentConnected(t *testing.T) {
	hub := startHub(t, testConfig(), AllowAll())

	ports := startForwarder(t, hub.Hub,
		Forward{Address: anyPort(), Agent: "agent-a", Target: Target{Service: "echo"}},
		Forward{Address: anyPort(), Target: Target{Service: "echo"}},
	)

	// the port stays open — it is the agent that is missing, not the listener
	for _, port := range ports {
		client, err := net.Dial("tcp", port)
		require.NoError(t, err)

		require.NoError(t, client.SetDeadline(time.Now().Add(5*time.Second)))
		_, err = client.Read(make([]byte, 1))
		assert.ErrorIs(t, err, io.EOF, "a port with no agent behind it should close what it accepts")

		client.Close()
	}
}

func TestForwardedPortCarriesAHalfClose(t *testing.T) {
	config := testConfig()
	hub := startHub(t, config, AllowAll())

	// the shape ssh and every request-then-answer protocol has: read until the
	// far end says it is finished asking, and only then answer.
	target := targetServer(t, func(conn net.Conn) {
		defer conn.Close()

		asked, err := io.ReadAll(conn)
		if err != nil {
			return
		}

		_, _ = conn.Write(append([]byte("answering "), asked...))
	})

	startAgent(t, "agent-a", []string{hub.address}, config,
		NewServiceTargets(map[string]string{"target": target}),
	)
	waitFor(t, "the agent", func() bool { return len(hub.Agents()) == 1 })

	ports := startForwarder(t, hub.Hub, Forward{
		Address: anyPort(),
		Agent:   "agent-a",
		Target:  Target{Service: "target"},
	})

	client, err := net.Dial("tcp", ports[0])
	require.NoError(t, err)
	defer client.Close()

	require.NoError(t, client.SetDeadline(time.Now().Add(10*time.Second)))
	_, err = io.WriteString(client, "the question")
	require.NoError(t, err)

	require.NoError(t, client.(*net.TCPConn).CloseWrite())

	answer, err := io.ReadAll(client)
	require.NoError(t, err)
	assert.Equal(t, "answering the question", string(answer))
}

func TestForwarderRefusesAPortItCannotHave(t *testing.T) {
	hub := startHub(t, testConfig(), AllowAll())

	taken, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer taken.Close()

	forwarder, err := NewForwarder(hub.Hub, discardLogger(),
		Forward{Address: anyPort(), Agent: "agent-a", Target: Target{Service: "echo"}},
		Forward{Address: taken.Addr().String(), Agent: "agent-a", Target: Target{Service: "echo"}},
	)
	require.NoError(t, err)

	assert.Error(t, forwarder.Listen(), "a port already taken should refuse to start")

	// and the one it did get is given back rather than left open
	assert.Empty(t, forwarder.Listening())
	require.NoError(t, forwarder.Close())
}

func TestForwarderRefusesWhatItCannotServe(t *testing.T) {
	hub := startHub(t, testConfig(), AllowAll())

	_, err := NewForwarder(nil, discardLogger())
	assert.Error(t, err, "a forwarder has nowhere to put a connection without a hub")

	_, err = NewForwarder(hub.Hub, discardLogger(), Forward{Address: anyPort()})
	assert.ErrorIs(t, err, ErrMalformedForward, "a forward that names no target cannot be served")
}

func TestForwarderShutdownLeavesNothingRunning(t *testing.T) {
	config := testConfig()
	hub := startHub(t, config, AllowAll())

	startAgent(t, "agent-a", []string{hub.address}, config,
		NewServiceTargets(map[string]string{"echo": echoServer(t, "")}),
	)
	waitFor(t, "the agent", func() bool { return len(hub.Agents()) == 1 })

	forwarder, err := NewForwarder(hub.Hub, discardLogger(), Forward{
		Address: anyPort(),
		Agent:   "agent-a",
		Target:  Target{Service: "echo"},
	})
	require.NoError(t, err)
	require.NoError(t, forwarder.Listen())

	served := make(chan error, 1)
	go func() { served <- forwarder.Serve(t.Context()) }()

	port := forwarder.Listening()[0].String()

	client, err := net.Dial("tcp", port)
	require.NoError(t, err)
	assert.Equal(t, "hello", roundTrip(t, client, "hello"))

	require.NoError(t, forwarder.Close())
	assert.NoError(t, <-served)

	// the port is gone, and closing twice is not an error
	_, err = net.Dial("tcp", port)
	assert.Error(t, err)
	assert.NoError(t, forwarder.Close())

	client.Close()
}
