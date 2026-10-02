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
// fabric's own directory, since it has to survive vmhost too, and is handed
// out by the fabric alone, since every machine on every network is vmhost's.
//
// The firewall is iptables chains of the fabric's own, written from the
// networks there are: machines reach their own network's neighbours; a public
// network reaches the internet, and nothing private or local on the way (10/8,
// 100.64/10, 127/8, 169.254/16, 172.16/12, 192.168/16), nor the blocked ports;
// machines on the public network do not reach each other there; nothing a
// machine starts reaches vmhost; and nothing a machine's VMM sends of its own,
// as its user, goes anywhere. The chains are read with iptables-save and
// written whole with iptables-restore, one transaction a table, and only when
// they are not as they should be: a firewall flushed and filled again rule by
// rule would let everything through while it was being filled, and Repair
// asks every few seconds.
//
// Ported from PR #101's hostnet, which ran the same bridges, taps and rules in
// its launcher's own namespace and was tested end to end, and its
// orchestrator's addresses; what is new is that networks are vmhost's alone
// (no owner in their names), the blocked ports, the bridge's own MAC, and the
// firewall written atomically and repaired.
package fabric

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"path/filepath"
	"slices"
	"sync"

	"go.opentelemetry.io/otel/metric"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

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
	// its taps. With no UIDs, every machine runs as vmhost itself, which is
	// for development and nothing else.
	FirstUID int
	UIDs     int
}

// Fabric is machines' networks, in the namespace vmhost runs in.
type Fabric struct {
	config Config
	logger *slog.Logger

	// tables is the namespace's firewall, read and written whole.
	tables tables

	// leases are the addresses handed out on every network.
	leases *leases

	// repairs counts the times the firewall was found changed by something
	// else and put back.
	repairs metric.Int64Counter

	// lock serialises everything the fabric does: allocating a network or an
	// address and writing the firewall all read what is there before they
	// change it.
	lock sync.Mutex
}

var _ vm.Fabric = (*Fabric)(nil)

// maxDevices is how many network devices a machine may have: a tap's name
// says which of its machine's devices it is in one hex digit.
const maxDevices = 16

// validate refuses a configuration the fabric cannot make networks from.
func (c Config) validate() error {
	if !filepath.IsAbs(c.Dir) {
		return fmt.Errorf("%q is not a directory the fabric can keep anything in: it has to be an absolute path", c.Dir)
	}

	if c.Pool == nil || c.Pool.IP.To4() == nil {
		return errors.New("machines' networks need an IPv4 pool to be carved out of")
	}

	if ones, bits := c.Pool.Mask.Size(); bits != 32 || ones > 24 {
		return fmt.Errorf("%s cannot be divided into /24s, one for each network", c.Pool)
	}

	if c.UIDs < 0 || c.FirstUID < 0 || int64(c.FirstUID)+int64(c.UIDs) > math.MaxUint32 {
		return fmt.Errorf("%d users from %d are not users machines can run as", c.UIDs, c.FirstUID)
	}

	// nothing a machine's user sends goes anywhere, so root cannot be one:
	// vmhost itself would send nothing.
	if c.UIDs > 0 && c.FirstUID == 0 {
		return errors.New("machines cannot run as root: their users send nothing of their own, and vmhost would not either")
	}

	return nil
}

// validatePlug refuses what cannot be plugged in as it is asked for.
func validatePlug(id string, attachments []vm.Attachment) error {
	if !vm.IsID(id) {
		return fmt.Errorf("%w: %q is not a machine", vm.ErrInvalid, id)
	}

	if len(attachments) > maxDevices {
		return fmt.Errorf("%w: a machine may join at most %d networks, not %d", vm.ErrInvalid, maxDevices, len(attachments))
	}

	var (
		joined   []string
		gateways int
	)

	for _, attachment := range attachments {
		if !vm.IsNetworkName(attachment.Network) {
			return fmt.Errorf("%w: %q is not a network", vm.ErrInvalid, attachment.Network)
		}

		// a second device on the same network would be given the same address.
		if slices.Contains(joined, attachment.Network) {
			return fmt.Errorf("%w: a machine joins %s once", vm.ErrInvalid, attachment.Network)
		}

		joined = append(joined, attachment.Network)

		if attachment.Gateway {
			gateways++
		}
	}

	if gateways > 1 {
		return fmt.Errorf("%w: a machine routes out through one network, not %d", vm.ErrInvalid, gateways)
	}

	return nil
}

// owner is the user a machine's taps are made for: the one its VMM runs as.
// It has to be one of the machines' users, which the firewall keeps from
// sending anything of their own, or, where machines run as vmhost itself,
// vmhost's own user, which zero stands for.
func (c Config) owner(uid int, self int) (int, error) {
	if c.UIDs == 0 {
		if uid != 0 {
			return 0, fmt.Errorf("%w: machines run as vmhost itself here, not as user %d", vm.ErrInvalid, uid)
		}

		return self, nil
	}

	if uid < c.FirstUID || uid >= c.FirstUID+c.UIDs {
		return 0, fmt.Errorf("%w: user %d is not one of the %d machines run as, from %d", vm.ErrInvalid, uid, c.UIDs, c.FirstUID)
	}

	return uid, nil
}
