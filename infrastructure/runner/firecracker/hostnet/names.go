// Package hostnet is the host's side of the runner's microVM networks.
//
// Each network is a bridge on the host with a /24 of its own out of a pool,
// and each of a machine's network devices is a tap plugged into one. Nothing
// about any of it is written down anywhere but the kernel: a bridge carries
// what it is for in its alias, so the launcher holding them can restart,
// or be replaced, and find everything it made where it left it.
//
// What may reach what is the firewall's to say, and it says it from the
// networks as they are: machines reach their own network's neighbours, a
// public network reaches the internet, and nothing reaches the host itself.
package hostnet

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
)

const (
	// bridgePrefix and tapPrefix are what every device of the runner's is
	// named with, which is how the firewall and the launcher tell them from
	// everybody else's. A device name is at most fifteen characters.
	bridgePrefix = "rnb"
	tapPrefix    = "rnt"

	// aliasPrefix starts the alias a bridge carries what it is for in.
	aliasPrefix = "runner"
)

// bridgeName names the bridge an orchestrator's network is.
func bridgeName(owner string, name string) string {
	return bridgePrefix + shortHash(owner+"/"+name)
}

// tapName names one of a machine's taps.
func tapName(id string, index int) string {
	return tapPrefixOf(id) + strconv.Itoa(index)
}

// tapPrefixOf is what every one of a machine's taps is named with.
func tapPrefixOf(id string) string {
	return tapPrefix + shortHash(id)
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))

	return hex.EncodeToString(sum[:4])
}

// alias is what a bridge says about itself.
func alias(owner string, name string, masquerade bool) string {
	return fmt.Sprintf("%s %s %s masquerade=%t", aliasPrefix, owner, name, masquerade)
}

// parseAlias reads back what a bridge says about itself.
func parseAlias(value string) (owner string, name string, masquerade bool, ok bool) {
	fields := strings.Fields(value)
	if len(fields) != 4 || fields[0] != aliasPrefix {
		return "", "", false, false
	}

	flag, found := strings.CutPrefix(fields[3], "masquerade=")
	if !found {
		return "", "", false, false
	}

	parsed, err := strconv.ParseBool(flag)
	if err != nil {
		return "", "", false, false
	}

	return fields[1], fields[2], parsed, true
}

// allocate picks the first /24 of pool that is not taken.
func allocate(pool *net.IPNet, taken []*net.IPNet) (*net.IPNet, error) {
	base := pool.IP.To4()
	ones, bits := pool.Mask.Size()

	if base == nil || bits != 32 || ones > 24 {
		return nil, fmt.Errorf("%s cannot be divided into /24s", pool)
	}

	blocks := 1 << (24 - ones)

	for block := 0; block < blocks; block++ {
		candidate := &net.IPNet{
			IP:   net.IPv4(base[0], base[1]+byte(block>>8), base[2]+byte(block&0xff), 0).To4(),
			Mask: net.CIDRMask(24, 32),
		}

		if !overlapsAny(candidate, taken) {
			return candidate, nil
		}
	}

	return nil, fmt.Errorf("every /24 of %s is taken", pool)
}

func overlapsAny(candidate *net.IPNet, taken []*net.IPNet) bool {
	for _, t := range taken {
		if t.Contains(candidate.IP) || candidate.Contains(t.IP) {
			return true
		}
	}

	return false
}

// gateway is the host's own address on a subnet: its first.
func gateway(subnet *net.IPNet) net.IP {
	ip := subnet.IP.To4()

	return net.IPv4(ip[0], ip[1], ip[2], ip[3]+1).To4()
}
