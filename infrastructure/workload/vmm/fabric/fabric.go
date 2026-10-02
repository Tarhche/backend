// Package fabric gives microVMs their networks: a bridge for each network,
// with a /24 of its own out of a pool, a tap for each of a machine's devices,
// an address for each tap, and the firewall that says what may reach what.
//
// All of it lives in one network namespace of vmhost's own, which outlives
// vmhost's container — it is a long-lived holder container's, which vmhost
// shares — so a vmhost that is redeployed finds every bridge, tap and rule
// where it left them, and the machines on them never notice. The host's own
// namespace, where docker keeps its firewall, is never touched: a machine's
// traffic leaves through the holder's own interface, as any container's does,
// and docker masquerades it as it would any container's.
//
// A bridge carries what it is for in its alias, so the kernel is the record of
// the networks; which address on which network is whose is kept in the
// fabric's own directory, since it has to survive vmhost too.
//
// The firewall is iptables chains of the fabric's own, flushed and written
// again from the networks there are: machines reach their own network's
// neighbours; a public network reaches the internet, and nothing private or
// local on the way (10/8, 100.64/10, 127/8, 169.254/16, 172.16/12,
// 192.168/16), nor the blocked ports; machines on the public network do not
// reach each other there; nothing a machine starts reaches vmhost; and nothing
// a machine's VMM sends of its own, as its user, goes anywhere.
package fabric

import (
	"errors"
	"log/slog"
	"net"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// errNotImplemented is what a stub answers.
var errNotImplemented = errors.New("fabric: not implemented yet")

// Config is what machines' networks are made of.
type Config struct {
	// Dir is where the fabric keeps what it has to remember (layout.Fabric).
	Dir string

	// Pool is where every network's /24 comes from. It must be the fabric's
	// alone.
	Pool *net.IPNet

	// BlockedPorts are ports no machine reaches on the internet.
	BlockedPorts []uint16

	// FirstUID and UIDs are the host users machines' VMMs run as. Nothing
	// they send of their own goes anywhere: what a machine says goes through
	// its taps.
	FirstUID int
	UIDs     int
}

// Fabric is machines' networks, in the namespace vmhost runs in.
type Fabric struct {
	config Config
	logger *slog.Logger
}

var _ vm.Fabric = (*Fabric)(nil)
