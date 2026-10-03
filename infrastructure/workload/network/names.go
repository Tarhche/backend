package network

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/khanzadimahdi/testproject/domain/workload/network"
)

// prefixPattern is what a network name may start with on a docker daemon,
// which allows letters, digits and a few separators, and has to begin with
// one of the first two.
var prefixPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,31}$`)

// Names is what one class calls the workload's networks on its daemon.
//
// The workload names its networks once, for every class: the isolated network
// standalone tasks share, and a private one per stack (domain/workload/network).
// Two classes on the same daemon — sysbox next to gvisor, both containers —
// must not share those bridges, or a stack of one would reach a stack of the
// other. So a class may be given a prefix, which is put in front of the
// workload's own networks on the way into docker and taken off again on the way
// out, and nothing above the driver ever sees it.
//
// Docker's own networks are docker's, whatever class asks for them: the
// default bridge, which is what a public task routes out through, and "none".
//
// No prefix is the names as they always were, which is what every network made
// before there were classes is called.
type Names struct {
	prefix string
}

// NewNames is the names of a class whose networks start with prefix.
func NewNames(prefix string) (Names, error) {
	if len(prefix) > 0 && !prefixPattern.MatchString(prefix) {
		return Names{}, fmt.Errorf("%q cannot start a docker network's name: it is letters, digits, '_', '.' and '-', and starts with a letter or a digit", prefix)
	}

	return Names{prefix: prefix}, nil
}

// Prefix is what this class's networks start with on the daemon.
func (n Names) Prefix() string {
	return n.prefix
}

// Docker is what the daemon calls a workload network.
func (n Names) Docker(name string) string {
	if len(n.prefix) == 0 || dockers(name) {
		return name
	}

	return n.prefix + name
}

// Workload is what the workload calls a network the daemon names, which is
// the same name with this class's prefix taken off.
func (n Names) Workload(name string) string {
	if len(n.prefix) == 0 || dockers(name) {
		return name
	}

	return strings.TrimPrefix(name, n.prefix)
}

// dockers reports whether a network is the daemon's own rather than one the
// workload made.
func dockers(name string) bool {
	return name == network.PublicNetworkName || name == network.NoNetworkName
}
