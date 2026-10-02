package driver

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
)

// The options a driver of either kind reads off its endpoint's query, so that
// one line of configuration says everything about a class.
const (
	// OptionOCIRuntime is the OCI runtime a container driver asks its daemon
	// for, such as sysbox-runc or runsc. Empty is the daemon's default.
	OptionOCIRuntime = "oci-runtime"

	// OptionAdvertiseHost is where a container driver reaches the ports its
	// daemon publishes, which is the daemon's host rather than the
	// orchestrator's.
	OptionAdvertiseHost = "advertise-host"

	// OptionNetworkPrefix is what a container driver names its networks
	// with, so that two classes on one daemon do not share bridges.
	OptionNetworkPrefix = "network-prefix"

	// OptionNodeMemory is the share of a vmhost's memory, in bytes, one
	// orchestrator may use, for when several share it.
	OptionNodeMemory = "node-memory"
)

// Spec is one class as an orchestrator is configured to offer it: one item of
// WORKLOAD_ORCHESTRATOR_RUNTIMES, written class=kind@endpoint, with the
// driver's options as the endpoint's query:
//
//	sysbox=container@tcp://docker:2375?oci-runtime=sysbox-runc
//	firecracker=microvm@unix:///run/workload-vmhost/vmhost.sock
type Spec struct {
	Class runtime.Class
	Kind  Kind

	// Endpoint is where the driver reaches what stands behind it, without
	// its options. A container driver's may be empty, which is the docker
	// client's own default.
	Endpoint string

	Options url.Values
}

// Option is one of the driver's options, or empty when it was not given.
func (s Spec) Option(name string) string {
	return s.Options.Get(name)
}

// String is the spec as it is written in the configuration.
func (s Spec) String() string {
	endpoint := s.Endpoint
	if encoded := s.Options.Encode(); len(encoded) > 0 {
		endpoint += "?" + encoded
	}

	return fmt.Sprintf("%s=%s@%s", s.Class, s.Kind, endpoint)
}

// kindPattern is what a driver kind can be called.
var kindPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// ParseSpecs reads WORKLOAD_ORCHESTRATOR_RUNTIMES: specs separated by commas,
// which is why no option may hold a comma. Empty is no spec at all; what an
// orchestrator offers then is its configuration's to say, and it is sysbox on
// DOCKER_HOST, as it was before there were classes.
//
// A class named twice is refused, since two drivers cannot both own a class.
// Whether a kind exists is not known here, but to the registry that builds
// the drivers, which refuses one it has no factory for.
func ParseSpecs(value string) ([]Spec, error) {
	specs := make([]Spec, 0, 2)
	seen := make(map[runtime.Class]bool)

	for item := range strings.SplitSeq(value, ",") {
		item = strings.TrimSpace(item)
		if len(item) == 0 {
			continue
		}

		spec, err := parseSpec(item)
		if err != nil {
			return nil, err
		}

		if seen[spec.Class] {
			return nil, fmt.Errorf("the %q runtime class is configured more than once", spec.Class)
		}

		seen[spec.Class] = true
		specs = append(specs, spec)
	}

	return specs, nil
}

func parseSpec(item string) (Spec, error) {
	class, rest, found := strings.Cut(item, "=")
	if !found {
		return Spec{}, fmt.Errorf("%q is not a runtime: it is written class=kind@endpoint", item)
	}

	kind, endpoint, found := strings.Cut(rest, "@")
	if !found {
		return Spec{}, fmt.Errorf("%q is not a runtime: it is written class=kind@endpoint", item)
	}

	spec := Spec{
		Class:   runtime.Class(strings.TrimSpace(class)),
		Kind:    Kind(strings.TrimSpace(kind)),
		Options: url.Values{},
	}

	if !spec.Class.IsValid() {
		return Spec{}, fmt.Errorf("%q cannot name a runtime class", spec.Class)
	}

	if !kindPattern.MatchString(string(spec.Kind)) {
		return Spec{}, fmt.Errorf("%q cannot name a driver kind", spec.Kind)
	}

	endpoint = strings.TrimSpace(endpoint)

	address, query, _ := strings.Cut(endpoint, "?")
	if len(query) > 0 {
		options, err := url.ParseQuery(query)
		if err != nil {
			return Spec{}, fmt.Errorf("the options of the %q runtime cannot be read: %w", spec.Class, err)
		}

		spec.Options = options
	}

	if len(address) > 0 {
		parsed, err := url.Parse(address)
		if err != nil {
			return Spec{}, fmt.Errorf("the endpoint of the %q runtime is not one: %w", spec.Class, err)
		}

		if len(parsed.Scheme) == 0 {
			return Spec{}, fmt.Errorf("the endpoint of the %q runtime names no scheme, such as unix:// or tcp://", spec.Class)
		}
	}

	spec.Endpoint = address

	return spec, nil
}
