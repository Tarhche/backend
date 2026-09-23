//go:build linux

package agent

import (
	"fmt"
	"net"
	"strings"

	"github.com/vishvananda/netlink"

	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/guest"
)

// configureInterfaces gives each of the machine's network devices the address
// it was told, and routes out through the one that was named the gateway.
// Devices are matched by MAC: which order the kernel finds them in is its own
// business.
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

func linkByMAC(links []netlink.Link, mac string) netlink.Link {
	for _, link := range links {
		if strings.EqualFold(link.Attrs().HardwareAddr.String(), mac) {
			return link
		}
	}

	return nil
}

// primaryAddress is the address the task's ports are reached on: its own on
// the first network it joined, which is the one its neighbours reach it on.
func primaryAddress(interfaces []guest.Interface) (string, bool) {
	if len(interfaces) == 0 {
		return "", false
	}

	address, _, _ := strings.Cut(interfaces[0].Address, "/")

	return address, len(address) > 0
}
