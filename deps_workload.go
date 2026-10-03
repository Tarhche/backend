//go:build tools

package main

// The modules the microVM runtime is built on, pinned here before the code
// that imports them exists, so that the branches building it in parallel share
// one go.mod rather than each adding to it: the firecracker SDK and its vsock
// dialer (vmhost's VMM and its guest client, and the agent's listener), systemd's
// D-Bus API (machines run as host units), go-containerregistry (images into
// disks), netlink and iptables (machines' networks), and pty (terminals in the
// guest).
//
// Nothing builds this file. Delete it once the code importing these is merged.
import (
	_ "github.com/coreos/go-iptables/iptables"
	_ "github.com/coreos/go-systemd/v22/dbus"
	_ "github.com/creack/pty"
	_ "github.com/firecracker-microvm/firecracker-go-sdk"
	_ "github.com/firecracker-microvm/firecracker-go-sdk/vsock"
	_ "github.com/google/go-containerregistry/pkg/authn"
	_ "github.com/google/go-containerregistry/pkg/name"
	_ "github.com/google/go-containerregistry/pkg/v1/mutate"
	_ "github.com/google/go-containerregistry/pkg/v1/remote"
	_ "github.com/vishvananda/netlink"
	_ "github.com/vishvananda/netns"
	_ "golang.org/x/sync/singleflight"
)
