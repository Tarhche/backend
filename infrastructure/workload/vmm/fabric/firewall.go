package fabric

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// The chains the fabric keeps its rules in. They are its own, written whole
// whenever they are not what the networks say they should be, and jumped to
// from the top of the chains the kernel runs, so nothing else has to be
// touched to change them.
const (
	forwardChain     = "WORKLOAD-FORWARD"
	inputChain       = "WORKLOAD-INPUT"
	outputChain      = "WORKLOAD-OUTPUT"
	postroutingChain = "WORKLOAD-POSTROUTING"

	// dockerUserChain is where docker lets rules of its own go ahead of its
	// own. Where docker keeps a firewall, forwarded traffic is dropped unless
	// something there lets it through first. The holder's namespace has none
	// of docker's chains, and its forwarded traffic is the fabric's alone.
	dockerUserChain = "DOCKER-USER"

	filterTable = "filter"
	natTable    = "nat"

	// multiportMax is how many ports one multiport rule takes.
	multiportMax = 15
)

// network is a network as the kernel holds it.
type network struct {
	name       string
	subnet     *net.IPNet
	masquerade bool
}

// view is a network as vmhost says it is.
func (n network) view() vm.Network {
	return vm.Network{
		Name:       n.name,
		Subnet:     n.subnet.String(),
		Gateway:    gateway(n.subnet).String(),
		Masquerade: n.masquerade,
	}
}

// rule is one rule of one chain, written the way iptables-save writes it back,
// so that what is there can be told from what should be by reading it.
type rule struct {
	table string
	chain string
	spec  []string
}

// deniedDestinations are what no machine reaches out to, whatever network it
// is on: everything private or local. That is the host's own networks, its
// LAN, a cloud's private networks and its metadata service, and every docker
// network, the platform's own services among them. The pool is in one of them,
// and what a network's machines say to each other is let through before these
// are reached. The pool is IPv4 alone, and IPv6 is off on every device of the
// fabric's, so there is nothing of IPv6 to deny.
var deniedDestinations = []string{
	"10.0.0.0/8",     // private
	"100.64.0.0/10",  // shared address space, which some clouds keep their own services in
	"127.0.0.0/8",    // loopback
	"169.254.0.0/16", // link-local, which a cloud's metadata service is on
	"172.16.0.0/12",  // private, and where docker's networks are
	"192.168.0.0/16", // private
}

// users are the users machines run as: the first of them, and how many.
type users struct {
	first int
	count int
}

// rules are what the firewall says about the pool and the networks in it.
//
// Every network is its own subnet of the pool. Machines on one reach each
// other there, and nothing else inside the pool; a public network also reaches
// out of the pool to the internet, masqueraded as the namespace, to nothing
// private or local on the way, and on none of the blocked ports. Machines on a
// public network are kept from each other rather than let through: all a
// public network gives a machine is the way out. Nothing of the pool reaches
// the namespace itself, where vmhost is, which answers only what the machines
// started. And a machine's own process sends nothing at all: what a machine
// says goes through its taps, never out of the namespace's own address.
//
// Traffic that is neither from the pool nor to it is left for the rest of the
// rules to decide.
func rules(pool *net.IPNet, networks []network, machines users, blocked []uint16) []rule {
	poolCIDR := pool.String()

	sorted := slices.Clone(networks)
	slices.SortFunc(sorted, func(a network, b network) int {
		return bytes.Compare(a.subnet.IP.To4(), b.subnet.IP.To4())
	})

	forward := []rule{
		{table: filterTable, chain: forwardChain, spec: []string{"-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"}},
	}

	var postrouting []rule

	for _, n := range sorted {
		subnet := n.subnet.String()

		if !n.masquerade {
			forward = append(forward,
				rule{table: filterTable, chain: forwardChain, spec: []string{"-s", subnet, "-d", subnet, "-j", "ACCEPT"}},
			)

			continue
		}

		forward = append(forward,
			rule{table: filterTable, chain: forwardChain, spec: []string{"-s", subnet, "-d", subnet, "-j", "DROP"}},
		)

		for _, denied := range deniedDestinations {
			forward = append(forward,
				rule{table: filterTable, chain: forwardChain, spec: []string{"-s", subnet, "-d", denied, "-j", "DROP"}},
			)
		}

		for _, ports := range portLists(blocked) {
			for _, protocol := range []string{"tcp", "udp"} {
				forward = append(forward,
					rule{table: filterTable, chain: forwardChain, spec: []string{"-s", subnet, "-p", protocol, "-m", "multiport", "--dports", ports, "-j", "DROP"}},
				)
			}
		}

		forward = append(forward,
			rule{table: filterTable, chain: forwardChain, spec: []string{"-s", subnet, "!", "-d", poolCIDR, "-j", "ACCEPT"}},
		)

		postrouting = append(postrouting,
			rule{table: natTable, chain: postroutingChain, spec: []string{"-s", subnet, "!", "-d", poolCIDR, "-j", "MASQUERADE"}},
		)
	}

	forward = append(forward,
		rule{table: filterTable, chain: forwardChain, spec: []string{"-s", poolCIDR, "-j", "DROP"}},
		rule{table: filterTable, chain: forwardChain, spec: []string{"-d", poolCIDR, "-j", "DROP"}},
	)

	input := []rule{
		{table: filterTable, chain: inputChain, spec: []string{"-i", bridgePrefix + "+", "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"}},
		{table: filterTable, chain: inputChain, spec: []string{"-i", bridgePrefix + "+", "-j", "DROP"}},
	}

	var output []rule
	if machines.count > 0 {
		owners := strconv.Itoa(machines.first) + "-" + strconv.Itoa(machines.first+machines.count-1)

		output = append(output,
			rule{table: filterTable, chain: outputChain, spec: []string{"-m", "owner", "--uid-owner", owners, "-j", "DROP"}},
		)
	}

	return slices.Concat(forward, input, output, postrouting)
}

// portLists are the blocked ports as multiport takes them: in order, each
// once, at most fifteen to a rule.
func portLists(blocked []uint16) []string {
	ports := slices.DeleteFunc(slices.Clone(blocked), func(port uint16) bool { return port == 0 })
	slices.Sort(ports)
	ports = slices.Compact(ports)

	var lists []string

	for chunk := range slices.Chunk(ports, multiportMax) {
		written := make([]string, len(chunk))
		for i, port := range chunk {
			written[i] = strconv.Itoa(int(port))
		}

		lists = append(lists, strings.Join(written, ","))
	}

	return lists
}

// chains are the fabric's own chains, by table, in the order they are written.
var chains = []struct {
	table string
	names []string
}{
	{table: filterTable, names: []string{forwardChain, inputChain, outputChain}},
	{table: natTable, names: []string{postroutingChain}},
}

// jump is where one of the fabric's chains is jumped to from.
type jump struct {
	table string
	from  string
	to    string
}

// jumps are where the fabric's chains are jumped to from: the first rule of
// each of the chains the kernel runs. Forwarded traffic goes through docker's
// own chain for rules like these when there is one, since docker drops what
// its own rules do not let through; in the holder's namespace there is none,
// and the kernel's chain is the one.
func jumps(dockerUser bool) []jump {
	forwardFrom := "FORWARD"
	if dockerUser {
		forwardFrom = dockerUserChain
	}

	return []jump{
		{table: filterTable, from: forwardFrom, to: forwardChain},
		{table: filterTable, from: "INPUT", to: inputChain},
		{table: filterTable, from: "OUTPUT", to: outputChain},
		{table: natTable, from: "POSTROUTING", to: postroutingChain},
	}
}

// ruleset is a firewall as iptables-save writes it: for each table, the
// chains it has and every chain's rules, in order, as their specs.
type ruleset map[string]*table

type table struct {
	chains map[string]bool
	rules  map[string][]string
}

// parseRuleset reads what iptables-save wrote.
func parseRuleset(saved string) ruleset {
	tables := ruleset{}

	var current *table

	for _, line := range strings.Split(saved, "\n") {
		line = strings.TrimSpace(line)

		switch {
		case line == "" || strings.HasPrefix(line, "#"):
		case strings.HasPrefix(line, "*"):
			current = &table{chains: map[string]bool{}, rules: map[string][]string{}}
			tables[strings.TrimPrefix(line, "*")] = current
		case current == nil:
		case line == "COMMIT":
			current = nil
		case strings.HasPrefix(line, ":"):
			name, _, _ := strings.Cut(strings.TrimPrefix(line, ":"), " ")
			current.chains[name] = true
		case strings.HasPrefix(line, "-A "):
			chain, spec, _ := strings.Cut(strings.TrimPrefix(line, "-A "), " ")
			current.rules[chain] = append(current.rules[chain], spec)
		}
	}

	return tables
}

func (r ruleset) has(table string, chain string) bool {
	t, found := r[table]

	return found && t.chains[chain]
}

func (r ruleset) rules(table string, chain string) []string {
	if t, found := r[table]; found {
		return t.rules[chain]
	}

	return nil
}

// plan is what has to be written for the firewall to be as desired, as
// iptables-restore --noflush takes it, and why; nothing, when it already is.
//
// The fabric's chains are written whole: declaring a chain to
// iptables-restore empties it, and the rules that follow fill it again, all in
// one transaction a table, so there is never a moment the chain holds less
// than it should. Jumps that are missing, or not first, are put first.
func plan(current ruleset, desired []rule, wanted []jump) (string, []string) {
	var reasons []string

	for _, group := range chains {
		for _, chain := range group.names {
			if !current.has(group.table, chain) {
				reasons = append(reasons, fmt.Sprintf("%s is not there", chain))

				continue
			}

			if !slices.Equal(current.rules(group.table, chain), specs(desired, group.table, chain)) {
				reasons = append(reasons, fmt.Sprintf("%s does not hold the rules it should", chain))
			}
		}
	}

	fixes := map[string][]string{}

	for _, j := range wanted {
		target := "-j " + j.to

		// a jump left where it no longer belongs, as one from FORWARD where
		// docker's own chain has since come, is taken away.
		for _, from := range []string{"FORWARD", dockerUserChain} {
			if from == j.from || j.to != forwardChain {
				continue
			}

			for _, spec := range current.rules(j.table, from) {
				if spec == target {
					reasons = append(reasons, fmt.Sprintf("%s is jumped to from %s, where it does not belong", j.to, from))
					fixes[j.table] = append(fixes[j.table], fmt.Sprintf("-D %s %s", from, target))
				}
			}
		}

		existing := current.rules(j.table, j.from)

		count := 0
		for _, spec := range existing {
			if spec == target {
				count++
			}
		}

		if count == 1 && existing[0] == target {
			continue
		}

		if count == 0 {
			reasons = append(reasons, fmt.Sprintf("%s is not jumped to from %s", j.to, j.from))
		} else {
			reasons = append(reasons, fmt.Sprintf("%s is not jumped to first from %s", j.to, j.from))
		}

		for range count {
			fixes[j.table] = append(fixes[j.table], fmt.Sprintf("-D %s %s", j.from, target))
		}

		fixes[j.table] = append(fixes[j.table], fmt.Sprintf("-I %s 1 %s", j.from, target))
	}

	if len(reasons) == 0 {
		return "", nil
	}

	return render(desired, fixes), reasons
}

// render writes the fabric's chains whole, and what else has to change, as
// iptables-restore --noflush takes it.
func render(desired []rule, fixes map[string][]string) string {
	var written strings.Builder

	for _, group := range chains {
		fmt.Fprintf(&written, "*%s\n", group.table)

		for _, chain := range group.names {
			fmt.Fprintf(&written, ":%s - [0:0]\n", chain)
		}

		for _, chain := range group.names {
			for _, spec := range specs(desired, group.table, chain) {
				fmt.Fprintf(&written, "-A %s %s\n", chain, spec)
			}
		}

		for _, fix := range fixes[group.table] {
			fmt.Fprintln(&written, fix)
		}

		fmt.Fprintln(&written, "COMMIT")
	}

	return written.String()
}

// specs are one chain's rules, as iptables-save writes them.
func specs(desired []rule, table string, chain string) []string {
	var written []string

	for _, r := range desired {
		if r.table == table && r.chain == chain {
			written = append(written, strings.Join(r.spec, " "))
		}
	}

	return written
}

// tables is a network namespace's firewall, read the way iptables-save
// writes it and written the way iptables-restore takes it.
type tables interface {
	save(ctx context.Context) (string, error)
	restore(ctx context.Context, input string) error
}

// apply makes the firewall what the networks say it should be, and says what
// it found different; nothing, when it changed nothing.
func (f *Fabric) apply(ctx context.Context, networks []network) ([]string, error) {
	saved, err := f.tables.save(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: the firewall cannot be read: %w", vm.ErrUnavailable, err)
	}

	current := parseRuleset(saved)
	desired := rules(f.config.Pool, networks, users{first: f.config.FirstUID, count: f.config.UIDs}, f.config.BlockedPorts)

	input, reasons := plan(current, desired, jumps(current.has(filterTable, dockerUserChain)))
	if len(reasons) == 0 {
		return nil, nil
	}

	if err := f.tables.restore(ctx, input); err != nil {
		return nil, fmt.Errorf("%w: the firewall cannot be written: %w", vm.ErrUnavailable, err)
	}

	return reasons, nil
}
