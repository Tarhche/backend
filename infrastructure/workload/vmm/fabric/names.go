package fabric

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// bridgePrefix and tapPrefix are what every device of the fabric's is
	// named with, which is how the firewall and the fabric tell them from
	// everybody else's. A device's name is at most fifteen characters, so
	// what follows the prefix is a hash, not the name of what it is for.
	bridgePrefix = "wkb"
	tapPrefix    = "wkt"

	// aliasPrefix starts the alias a bridge carries what it is for in, and
	// tapAliasPrefix the one a tap carries whose it is in.
	aliasPrefix    = "workload"
	tapAliasPrefix = "workload-vm"
)

// bridgeName names the bridge a network is: wkb and twelve hex digits.
func bridgeName(network string) string {
	return bridgePrefix + shortHash(network, 12)
}

// tapName names one of a machine's taps: its prefix, and which of the
// machine's devices it is, in one hex digit.
func tapName(id string, index int) string {
	return tapPrefixOf(id) + strconv.FormatInt(int64(index), 16)
}

// tapPrefixOf is what every one of a machine's taps is named with: wkt and
// eleven hex digits of a hash of its ID, which is what finds them again.
func tapPrefixOf(id string) string {
	return tapPrefix + shortHash(id, 11)
}

// isTap reports whether a device is one of the fabric's taps.
func isTap(device string) bool {
	return strings.HasPrefix(device, tapPrefix) && len(device) == len(tapPrefixOf("0"))+1
}

func shortHash(value string, length int) string {
	sum := sha256.Sum256([]byte(value))

	return hex.EncodeToString(sum[:])[:length]
}

// alias is what a bridge says about itself: what network it is, and whether
// it routes out.
func alias(network string, masquerade bool) string {
	return fmt.Sprintf("%s %s masquerade=%t", aliasPrefix, network, masquerade)
}

// parseAlias reads back what a bridge says about itself.
func parseAlias(value string) (network string, masquerade bool, ok bool) {
	fields := strings.Fields(value)
	if len(fields) != 3 || fields[0] != aliasPrefix || !vm.IsNetworkName(fields[1]) {
		return "", false, false
	}

	flag, found := strings.CutPrefix(fields[2], "masquerade=")
	if !found {
		return "", false, false
	}

	parsed, err := strconv.ParseBool(flag)
	if err != nil {
		return "", false, false
	}

	return fields[1], parsed, true
}

// tapAlias is what a tap says about itself, for whoever looks at the
// namespace: the machine it is for, and the network it is on.
func tapAlias(id string, network string) string {
	return fmt.Sprintf("%s %s %s", tapAliasPrefix, id, network)
}

// allocate picks a /24 of pool that is not taken: the one asked for when it
// is free, which keeps a network made again on the subnet it had, or the
// first that is.
func allocate(pool *net.IPNet, taken []*net.IPNet, preferred *net.IPNet) (*net.IPNet, error) {
	base := pool.IP.To4()
	ones, bits := pool.Mask.Size()

	if base == nil || bits != 32 || ones > 24 {
		return nil, fmt.Errorf("%s cannot be divided into /24s", pool)
	}

	if preferred != nil {
		if size, _ := preferred.Mask.Size(); size == 24 && pool.Contains(preferred.IP) && preferred.IP.Equal(preferred.IP.Mask(preferred.Mask)) && !overlapsAny(preferred, taken) {
			return &net.IPNet{IP: preferred.IP.To4(), Mask: net.CIDRMask(24, 32)}, nil
		}
	}

	start := binary.BigEndian.Uint32(base.Mask(pool.Mask))

	for block := uint32(0); block < 1<<(24-ones); block++ {
		ip := make(net.IP, net.IPv4len)
		binary.BigEndian.PutUint32(ip, start+block<<8)

		candidate := &net.IPNet{IP: ip, Mask: net.CIDRMask(24, 32)}
		if !overlapsAny(candidate, taken) {
			return candidate, nil
		}
	}

	return nil, fmt.Errorf("%w: every /24 of %s is taken by a network", vm.ErrCapacity, pool)
}

func overlapsAny(candidate *net.IPNet, taken []*net.IPNet) bool {
	for _, t := range taken {
		if t.Contains(candidate.IP) || candidate.Contains(t.IP) {
			return true
		}
	}

	return false
}

// gateway is the fabric's own address on a subnet, which its machines route
// through: its first.
func gateway(subnet *net.IPNet) net.IP {
	ip := make(net.IP, net.IPv4len)
	binary.BigEndian.PutUint32(ip, binary.BigEndian.Uint32(subnet.IP.To4())+1)

	return ip
}

// macOf is the MAC a device with address is given: a locally administered
// one, which is what a MAC nobody assigned has to be, carrying the address
// itself, so that no two devices on any of the fabric's networks share one.
// A bridge is given its gateway's: left to itself, it would take the lowest
// of its ports' MACs, and change it whenever a machine came or went, under
// its machines' ARP caches.
func macOf(address net.IP) net.HardwareAddr {
	ip := address.To4()

	return net.HardwareAddr{0x06, 0x00, ip[0], ip[1], ip[2], ip[3]}
}
