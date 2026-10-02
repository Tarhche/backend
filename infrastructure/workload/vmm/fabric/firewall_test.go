package fabric

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

var update = flag.Bool("update", false, "write the golden files again from what the code makes")

// blockedPorts are vmhost's default blocked ports (WORKLOAD_VMHOST_BLOCKED_EGRESS_PORTS).
var blockedPorts = []uint16{3333, 4444, 5555, 7777, 14433, 14444, 45560, 45700}

// fixture is a vmhost as it usually is: the public network, the isolated one
// standalone tasks join, and one stack's.
func fixture(t *testing.T) (*net.IPNet, []network) {
	t.Helper()

	return cidr(t, "10.250.0.0/16"), []network{
		{name: "workload-stack-shop", subnet: cidr(t, "10.250.2.0/24")},
		{name: vm.PublicNetwork, subnet: cidr(t, "10.250.0.0/24"), masquerade: true},
		{name: "workload-isolated", subnet: cidr(t, "10.250.1.0/24")},
	}
}

// TestFirewallGolden pins the exact rules the fabric writes, as
// iptables-restore takes them, so that a change to them is seen in review.
// Run with -update to write the golden files again.
func TestFirewallGolden(t *testing.T) {
	t.Parallel()

	pool, networks := fixture(t)

	for _, golden := range []struct {
		file       string
		machines   users
		dockerUser bool
	}{
		{file: "firewall.golden", machines: users{first: 1_000_000_000, count: 65536}},
		{file: "firewall-docker-user.golden", machines: users{first: 1_000_000_000, count: 65536}, dockerUser: true},
		{file: "firewall-development.golden"},
	} {
		t.Run(golden.file, func(t *testing.T) {
			t.Parallel()

			current := ruleset{}
			if golden.dockerUser {
				current = parseRuleset("*filter\n:FORWARD DROP [0:0]\n:DOCKER-USER - [0:0]\n-A FORWARD -j DOCKER-USER\n-A DOCKER-USER -j RETURN\nCOMMIT\n")
			}

			written, reasons := plan(current, rules(pool, networks, golden.machines, blockedPorts), jumps(golden.dockerUser))
			require.NotEmpty(t, reasons)

			path := filepath.Join("testdata", golden.file)

			if *update {
				require.NoError(t, os.WriteFile(path, []byte(written), 0o644))
			}

			expected, err := os.ReadFile(path)
			require.NoError(t, err)

			assert.Equal(t, string(expected), written)
		})
	}
}

func TestRules(t *testing.T) {
	t.Parallel()

	pool, networks := fixture(t)

	specsOf := func(rules []rule, chain string) []string {
		return specs(rules, tableOf(chain), chain)
	}

	t.Run("machines reach their own network's neighbours, and a public network reaches out to the internet alone", func(t *testing.T) {
		t.Parallel()

		forward := specsOf(rules(pool, networks, users{}, nil), forwardChain)

		assert.Equal(t, []string{
			"-m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT",
			"-s 10.250.0.0/24 -d 10.250.0.0/24 -j DROP",
			"-s 10.250.0.0/24 -d 10.0.0.0/8 -j DROP",
			"-s 10.250.0.0/24 -d 100.64.0.0/10 -j DROP",
			"-s 10.250.0.0/24 -d 127.0.0.0/8 -j DROP",
			"-s 10.250.0.0/24 -d 169.254.0.0/16 -j DROP",
			"-s 10.250.0.0/24 -d 172.16.0.0/12 -j DROP",
			"-s 10.250.0.0/24 -d 192.168.0.0/16 -j DROP",
			"-s 10.250.0.0/24 ! -d 10.250.0.0/16 -j ACCEPT",
			"-s 10.250.1.0/24 -d 10.250.1.0/24 -j ACCEPT",
			"-s 10.250.2.0/24 -d 10.250.2.0/24 -j ACCEPT",
			"-s 10.250.0.0/16 -j DROP",
			"-d 10.250.0.0/16 -j DROP",
		}, forward)
	})

	t.Run("the blocked ports are dropped on the way out, for TCP and UDP, fifteen to a rule", func(t *testing.T) {
		t.Parallel()

		var many []uint16
		for port := uint16(20); port > 0; port-- {
			many = append(many, port+9000, port+9000)
		}

		forward := specsOf(rules(pool, networks, users{}, append(many, 0)), forwardChain)

		assert.Contains(t, forward, "-s 10.250.0.0/24 -p tcp -m multiport --dports 9001,9002,9003,9004,9005,9006,9007,9008,9009,9010,9011,9012,9013,9014,9015 -j DROP")
		assert.Contains(t, forward, "-s 10.250.0.0/24 -p udp -m multiport --dports 9001,9002,9003,9004,9005,9006,9007,9008,9009,9010,9011,9012,9013,9014,9015 -j DROP")
		assert.Contains(t, forward, "-s 10.250.0.0/24 -p tcp -m multiport --dports 9016,9017,9018,9019,9020 -j DROP")
		assert.Contains(t, forward, "-s 10.250.0.0/24 -p udp -m multiport --dports 9016,9017,9018,9019,9020 -j DROP")

		blocked := slices.IndexFunc(forward, func(spec string) bool { return strings.Contains(spec, "multiport") })
		accepted := slices.Index(forward, "-s 10.250.0.0/24 ! -d 10.250.0.0/16 -j ACCEPT")
		assert.Less(t, blocked, accepted, "before the way out is let through")

		for _, spec := range forward {
			if strings.Contains(spec, "multiport") {
				assert.True(t, strings.HasPrefix(spec, "-s 10.250.0.0/24 "), "only what routes out is held to them: %s", spec)
			}
		}
	})

	t.Run("nothing a machine's own process sends goes anywhere", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, []string{"-m owner --uid-owner 1000000000-1000065535 -j DROP"}, specsOf(rules(pool, networks, users{first: 1_000_000_000, count: 65536}, nil), outputChain))
		assert.Empty(t, specsOf(rules(pool, networks, users{}, nil), outputChain), "machines that run as vmhost itself are not told apart from it")
	})

	t.Run("only a public network is masqueraded", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, []string{"-s 10.250.0.0/24 ! -d 10.250.0.0/16 -j MASQUERADE"}, specsOf(rules(pool, networks, users{}, nil), postroutingChain))

		isolatedOnly := []network{{name: "workload-isolated", subnet: cidr(t, "10.250.1.0/24")}}
		all := rules(pool, isolatedOnly, users{}, blockedPorts)

		assert.Empty(t, specsOf(all, postroutingChain))

		for _, spec := range specsOf(all, forwardChain) {
			assert.NotContains(t, spec, "! -d", "nothing is let out of the pool: %s", spec)
		}
	})

	t.Run("nothing a machine starts reaches the namespace itself", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, []string{
			"-i wkb+ -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT",
			"-i wkb+ -j DROP",
		}, specsOf(rules(pool, nil, users{}, nil), inputChain))
	})
}

func TestPlan(t *testing.T) {
	t.Parallel()

	pool, networks := fixture(t)
	desired := rules(pool, networks, users{first: 1_000_000_000, count: 65536}, blockedPorts)

	written := func(t *testing.T, firewall *fakeTables) ruleset {
		t.Helper()

		saved, err := firewall.save(context.Background())
		require.NoError(t, err)

		return parseRuleset(saved)
	}

	t.Run("a firewall as it should be is left as it is", func(t *testing.T) {
		t.Parallel()

		firewall := newFakeTables()
		input, _ := plan(ruleset{}, desired, jumps(false))
		require.NoError(t, firewall.restore(context.Background(), input))

		again, reasons := plan(written(t, firewall), desired, jumps(false))

		assert.Empty(t, again)
		assert.Empty(t, reasons)
	})

	t.Run("a chain flushed by something else is written whole again", func(t *testing.T) {
		t.Parallel()

		firewall := newFakeTables()
		input, _ := plan(ruleset{}, desired, jumps(false))
		require.NoError(t, firewall.restore(context.Background(), input))

		firewall.flush(filterTable, forwardChain)

		repaired, reasons := plan(written(t, firewall), desired, jumps(false))

		assert.Equal(t, []string{"WORKLOAD-FORWARD does not hold the rules it should"}, reasons)
		require.NoError(t, firewall.restore(context.Background(), repaired))

		_, reasons = plan(written(t, firewall), desired, jumps(false))
		assert.Empty(t, reasons, "and is as it should be afterwards")
	})

	t.Run("a jump that is not first is put first, and only once", func(t *testing.T) {
		t.Parallel()

		firewall := newFakeTables()
		input, _ := plan(ruleset{}, desired, jumps(false))
		require.NoError(t, firewall.restore(context.Background(), input))

		firewall.insert(filterTable, "FORWARD", "-j ACCEPT")
		firewall.insert(filterTable, "INPUT", "-j "+inputChain)

		repaired, reasons := plan(written(t, firewall), desired, jumps(false))

		assert.ElementsMatch(t, []string{
			"WORKLOAD-FORWARD is not jumped to first from FORWARD",
			"WORKLOAD-INPUT is not jumped to first from INPUT",
		}, reasons)
		assert.Contains(t, repaired, "-D FORWARD -j WORKLOAD-FORWARD\n-I FORWARD 1 -j WORKLOAD-FORWARD\n")
		assert.Contains(t, repaired, "-D INPUT -j WORKLOAD-INPUT\n-D INPUT -j WORKLOAD-INPUT\n-I INPUT 1 -j WORKLOAD-INPUT\n")

		require.NoError(t, firewall.restore(context.Background(), repaired))

		assert.Equal(t, []string{"-j WORKLOAD-FORWARD", "-j ACCEPT"}, firewall.rules(filterTable, "FORWARD"))
		assert.Equal(t, []string{"-j WORKLOAD-INPUT"}, firewall.rules(filterTable, "INPUT"))
	})

	t.Run("forwarded traffic goes through docker's own chain for rules like these, when docker is there", func(t *testing.T) {
		t.Parallel()

		firewall := newFakeTables()
		input, _ := plan(ruleset{}, desired, jumps(false))
		require.NoError(t, firewall.restore(context.Background(), input))

		// docker came after the fabric did.
		firewall.declare(filterTable, dockerUserChain)
		firewall.append(filterTable, dockerUserChain, "-j RETURN")
		firewall.insert(filterTable, "FORWARD", "-j "+dockerUserChain)

		current := written(t, firewall)
		repaired, reasons := plan(current, desired, jumps(current.has(filterTable, dockerUserChain)))

		assert.ElementsMatch(t, []string{
			"WORKLOAD-FORWARD is jumped to from FORWARD, where it does not belong",
			"WORKLOAD-FORWARD is not jumped to from DOCKER-USER",
		}, reasons)

		require.NoError(t, firewall.restore(context.Background(), repaired))

		assert.Equal(t, []string{"-j DOCKER-USER"}, firewall.rules(filterTable, "FORWARD"))
		assert.Equal(t, []string{"-j WORKLOAD-FORWARD", "-j RETURN"}, firewall.rules(filterTable, dockerUserChain))
	})
}

func TestApply(t *testing.T) {
	t.Parallel()

	pool, networks := fixture(t)

	newTestFabric := func(t *testing.T, firewall tables) *Fabric {
		t.Helper()

		book, err := openLeases(t.TempDir())
		require.NoError(t, err)

		return &Fabric{
			config: Config{Dir: t.TempDir(), Pool: pool, BlockedPorts: blockedPorts, FirstUID: 1_000_000_000, UIDs: 65536},
			logger: slog.New(slog.DiscardHandler),
			tables: firewall,
			leases: book,
		}
	}

	t.Run("the firewall is written once, and left alone while it is as it should be", func(t *testing.T) {
		t.Parallel()

		firewall := newFakeTables()
		fabric := newTestFabric(t, firewall)

		reasons, err := fabric.apply(context.Background(), networks)
		require.NoError(t, err)
		assert.NotEmpty(t, reasons)

		reasons, err = fabric.apply(context.Background(), networks)
		require.NoError(t, err)
		assert.Empty(t, reasons)

		assert.Len(t, firewall.restores, 1, "a firewall as it should be is not written again")
	})

	t.Run("a network that comes is written into the firewall", func(t *testing.T) {
		t.Parallel()

		firewall := newFakeTables()
		fabric := newTestFabric(t, firewall)

		_, err := fabric.apply(context.Background(), networks[:1])
		require.NoError(t, err)

		reasons, err := fabric.apply(context.Background(), networks)
		require.NoError(t, err)

		assert.Equal(t, []string{"WORKLOAD-FORWARD does not hold the rules it should", "WORKLOAD-POSTROUTING does not hold the rules it should"}, reasons)
		assert.Contains(t, firewall.rules(natTable, postroutingChain), "-s 10.250.0.0/24 ! -d 10.250.0.0/16 -j MASQUERADE")
	})

	t.Run("a firewall that cannot be read or written leaves vmhost unable to run machines", func(t *testing.T) {
		t.Parallel()

		unreadable := newFakeTables()
		unreadable.saveErr = errors.New("iptables-save: not found")

		_, err := newTestFabric(t, unreadable).apply(context.Background(), networks)
		assert.ErrorIs(t, err, vm.ErrUnavailable)

		unwritable := newFakeTables()
		unwritable.restoreErr = errors.New("iptables-restore: line 4 failed")

		_, err = newTestFabric(t, unwritable).apply(context.Background(), networks)
		assert.ErrorIs(t, err, vm.ErrUnavailable)
	})
}

func TestParseRuleset(t *testing.T) {
	t.Parallel()

	saved := `# Generated by iptables-nft-save v1.8.10 (nf_tables) on Fri Oct  2 16:00:00 2026
*nat
:PREROUTING ACCEPT [0:0]
:INPUT ACCEPT [0:0]
:OUTPUT ACCEPT [0:0]
:POSTROUTING ACCEPT [0:0]
:DOCKER_OUTPUT - [0:0]
:WORKLOAD-POSTROUTING - [0:0]
-A OUTPUT -d 127.0.0.11/32 -j DOCKER_OUTPUT
-A POSTROUTING -j WORKLOAD-POSTROUTING
-A WORKLOAD-POSTROUTING -s 10.250.0.0/24 ! -d 10.250.0.0/16 -j MASQUERADE
COMMIT
# Completed on Fri Oct  2 16:00:00 2026
*filter
:INPUT ACCEPT [0:0]
:FORWARD ACCEPT [0:0]
:OUTPUT ACCEPT [0:0]
:WORKLOAD-FORWARD - [0:0]
-A FORWARD -j WORKLOAD-FORWARD
-A WORKLOAD-FORWARD -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
COMMIT
# Warning: iptables-legacy tables present, use iptables-legacy-save to see them
`

	parsed := parseRuleset(saved)

	assert.True(t, parsed.has(natTable, postroutingChain))
	assert.True(t, parsed.has(filterTable, "FORWARD"))
	assert.False(t, parsed.has(filterTable, inputChain))
	assert.False(t, parsed.has("mangle", "PREROUTING"))
	assert.Equal(t, []string{"-j WORKLOAD-POSTROUTING"}, parsed.rules(natTable, "POSTROUTING"))
	assert.Equal(t, []string{"-m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT"}, parsed.rules(filterTable, forwardChain))
	assert.Empty(t, parsed.rules(filterTable, outputChain))
}

func tableOf(chain string) string {
	if chain == postroutingChain {
		return natTable
	}

	return filterTable
}

// fakeTables is a firewall that takes what iptables-restore --noflush takes,
// and writes back what iptables-save would: enough of iptables to see what
// the fabric writes do what it is meant to.
type fakeTables struct {
	tables   map[string]*fakeTable
	restores []string

	saveErr    error
	restoreErr error
}

type fakeTable struct {
	chains []string
	rules  map[string][]string
}

// builtins are the chains the kernel runs, which are there without being
// declared.
var builtins = map[string][]string{
	filterTable: {"INPUT", "FORWARD", "OUTPUT"},
	natTable:    {"PREROUTING", "INPUT", "OUTPUT", "POSTROUTING"},
}

func newFakeTables() *fakeTables {
	return &fakeTables{tables: map[string]*fakeTable{}}
}

func (f *fakeTables) table(name string) *fakeTable {
	if t, found := f.tables[name]; found {
		return t
	}

	t := &fakeTable{chains: slices.Clone(builtins[name]), rules: map[string][]string{}}
	f.tables[name] = t

	return t
}

func (f *fakeTables) declare(table string, chain string) {
	t := f.table(table)
	if !slices.Contains(t.chains, chain) {
		t.chains = append(t.chains, chain)
	}

	t.rules[chain] = nil
}

func (f *fakeTables) flush(table string, chain string) {
	f.table(table).rules[chain] = nil
}

func (f *fakeTables) append(table string, chain string, spec string) {
	t := f.table(table)
	t.rules[chain] = append(t.rules[chain], spec)
}

func (f *fakeTables) insert(table string, chain string, spec string) {
	t := f.table(table)
	t.rules[chain] = append([]string{spec}, t.rules[chain]...)
}

func (f *fakeTables) rules(table string, chain string) []string {
	return f.table(table).rules[chain]
}

func (f *fakeTables) save(ctx context.Context) (string, error) {
	if f.saveErr != nil {
		return "", f.saveErr
	}

	names := make([]string, 0, len(f.tables))
	for name := range f.tables {
		names = append(names, name)
	}

	sort.Strings(names)

	var saved strings.Builder
	fmt.Fprintln(&saved, "# Generated by the fake")

	for _, name := range names {
		t := f.tables[name]
		fmt.Fprintf(&saved, "*%s\n", name)

		for _, chain := range t.chains {
			policy := "-"
			if slices.Contains(builtins[name], chain) {
				policy = "ACCEPT"
			}

			fmt.Fprintf(&saved, ":%s %s [0:0]\n", chain, policy)
		}

		for _, chain := range t.chains {
			for _, spec := range t.rules[chain] {
				fmt.Fprintf(&saved, "-A %s %s\n", chain, spec)
			}
		}

		fmt.Fprintln(&saved, "COMMIT")
	}

	return saved.String(), nil
}

func (f *fakeTables) restore(ctx context.Context, input string) error {
	if f.restoreErr != nil {
		return f.restoreErr
	}

	f.restores = append(f.restores, input)

	var current string

	for number, line := range strings.Split(input, "\n") {
		fail := func(reason string) error {
			return fmt.Errorf("line %d (%q): %s", number+1, line, reason)
		}

		switch {
		case line == "":
		case strings.HasPrefix(line, "*"):
			current = strings.TrimPrefix(line, "*")
			f.table(current)
		case line == "COMMIT":
			current = ""
		case current == "":
			return fail("outside a table")
		case strings.HasPrefix(line, ":"):
			chain, _, _ := strings.Cut(strings.TrimPrefix(line, ":"), " ")
			f.declare(current, chain)
		default:
			fields := strings.Fields(line)
			if len(fields) < 2 {
				return fail("not a rule")
			}

			chain := fields[1]
			if !slices.Contains(f.table(current).chains, chain) {
				return fail("no such chain")
			}

			switch fields[0] {
			case "-A":
				f.append(current, chain, strings.Join(fields[2:], " "))
			case "-I":
				if len(fields) < 3 || fields[2] != "1" {
					return fail("only inserting first is understood")
				}

				f.insert(current, chain, strings.Join(fields[3:], " "))
			case "-D":
				spec := strings.Join(fields[2:], " ")
				rules := f.table(current).rules[chain]

				at := slices.Index(rules, spec)
				if at < 0 {
					return fail("no such rule")
				}

				f.table(current).rules[chain] = slices.Delete(rules, at, at+1)
			default:
				return fail("not understood")
			}
		}
	}

	return nil
}
