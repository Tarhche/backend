//go:build linux

package agent

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/vishvananda/netlink"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
)

// configureInterfaces gives each of the machine's network devices the address
// it was told, and routes out through the one that was named the gateway.
// Devices are matched by MAC: which order the kernel finds them in is its own
// business.
//
// A machine's networks carry IPv4 alone, and drop anything else, so IPv6 is
// turned off on its devices before they come up: they then claim no address
// nothing would route, and a lookup does not hand the task one it cannot
// reach. The loopback keeps ::1, as a container's does.
func configureInterfaces(interfaces []guest.Interface) error {
	links, err := netlink.LinkList()
	if err != nil {
		return err
	}

	for _, i := range interfaces {
		link := linkByMAC(links, i.MAC)
		if link == nil {
			return fmt.Errorf("the machine has no network device with MAC %s", i.MAC)
		}

		address, err := netlink.ParseAddr(i.Address)
		if err != nil {
			return fmt.Errorf("%q is not an address: %w", i.Address, err)
		}

		disableIPv6(link.Attrs().Name)

		if err := netlink.AddrReplace(link, address); err != nil {
			return err
		}

		if err := netlink.LinkSetUp(link); err != nil {
			return err
		}

		if len(i.Gateway) == 0 {
			continue
		}

		gateway := net.ParseIP(i.Gateway)
		if gateway == nil {
			return fmt.Errorf("%q is not a gateway", i.Gateway)
		}

		if err := netlink.RouteReplace(&netlink.Route{LinkIndex: link.Attrs().Index, Gw: gateway}); err != nil {
			return fmt.Errorf("failed to route out through %s: %w", i.Gateway, err)
		}
	}

	return nil
}

// disableIPv6 turns IPv6 off on one device. A kernel without IPv6 has nothing
// to turn off.
func disableIPv6(name string) {
	_ = os.WriteFile(filepath.Join("/proc/sys/net/ipv6/conf", name, "disable_ipv6"), []byte("1"), 0o644)
}

func linkByMAC(links []netlink.Link, mac string) netlink.Link {
	for _, link := range links {
		if strings.EqualFold(link.Attrs().HardwareAddr.String(), mac) {
			return link
		}
	}

	return nil
}
