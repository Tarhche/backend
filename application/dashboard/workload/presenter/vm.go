package presenter

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// VM is one VM, as the dashboard shows it.
type VM struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`

	// Slug is the name its ports are served under.
	Slug string `json:"slug"`

	OwnerUUID string `json:"owner_uuid"`
	Owner     *Owner `json:"owner,omitempty"`

	Kind  string `json:"kind"`
	Image string `json:"image"`

	Resources Resources `json:"resources"`

	// Ports are the guest ports the ingress serves, sorted.
	Ports   []uint  `json:"ports"`
	Network Network `json:"network"`

	PersistentDisk bool `json:"persistent_disk"`

	// LifetimeSeconds is how long the VM is kept, and zero is until it is
	// deleted. ExpiresAt is when one with a lifetime goes.
	LifetimeSeconds int64      `json:"lifetime_seconds"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`

	// State is what the VM is doing; ExpectedState is what it was asked to be
	// doing. They differ while the workload is closing the gap.
	State         string `json:"state"`
	ExpectedState string `json:"expected_state,omitempty"`

	// Reason is why it failed, or what is pending, when the workload can say.
	Reason   string `json:"reason,omitempty"`
	NodeName string `json:"node_name,omitempty"`

	// Stats is the last sample its node reported, and is left out until there
	// is one.
	Stats *Stats `json:"stats,omitempty"`

	// URLs are where its ports are served, while its ingress allows it.
	URLs []URL `json:"urls"`

	CreatedAt time.Time  `json:"created_at"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`

	// ManagedBy is code-runner for a snippet the code runner is running, in a
	// VM of its own for as long as it runs: the guest's, listed only among
	// everybody's VMs, and gone once the snippet has ended. Such a VM can be
	// stopped, deleted and read, and nothing else. It is left out for every
	// other VM.
	ManagedBy string `json:"managed_by,omitempty" enums:"code-runner"`
}

// Resources are whole vCPUs, and bytes of memory and disk.
type Resources struct {
	CPUs   uint   `json:"cpus"`
	Memory uint64 `json:"memory"`
	Disk   uint64 `json:"disk"`
}

// Network is whether anything may reach the VM's ports, and whether the VM may
// reach out: allow or deny each way.
type Network struct {
	Ingress string `json:"ingress"`
	Egress  string `json:"egress"`
}

// Stats is one sample of what a VM uses. Memory and disk are bytes, and the
// network counters are bytes since the VM started.
type Stats struct {
	// CPUPercent is 0 to 100 of all of the VM's vCPUs together: 100 is every
	// one of them busy, however many it has.
	CPUPercent  float64   `json:"cpu_percent"`
	MemoryUsed  uint64    `json:"memory_used"`
	MemoryLimit uint64    `json:"memory_limit"`
	DiskUsed    uint64    `json:"disk_used"`
	DiskTotal   uint64    `json:"disk_total"`
	NetworkRx   uint64    `json:"network_rx"`
	NetworkTx   uint64    `json:"network_tx"`
	SampledAt   time.Time `json:"sampled_at"`
}

// URL is one of a VM's ports, and the address it answers on.
type URL struct {
	Port uint   `json:"port"`
	URL  string `json:"url"`
}

// NewVM presents one VM. The ingress domain is what its addresses are built
// from, so the dashboard can link straight to it.
func NewVM(v vm.VM, ingressDomain string, owners Owners) VM {
	ports := make([]uint, len(v.Ports))
	for i, p := range v.Ports {
		ports[i] = uint(p)
	}

	return VM{
		UUID:      v.UUID,
		Name:      v.Name,
		Slug:      v.Slug,
		OwnerUUID: v.OwnerUUID,
		Owner:     owners.Of(v.OwnerUUID),
		Kind:      v.Kind.String(),
		Image:     v.Image,
		Resources: Resources{
			CPUs:   v.Resources.CPUs,
			Memory: v.Resources.Memory,
			Disk:   v.Resources.Disk,
		},
		Ports: ports,
		Network: Network{
			Ingress: v.Network.Ingress.String(),
			Egress:  v.Network.Egress.String(),
		},
		PersistentDisk:  v.PersistentDisk,
		LifetimeSeconds: int64(v.Lifetime / time.Second),
		ExpiresAt:       when(v.ExpiresAt),
		State:           vmState(v.CurrentState),
		ExpectedState:   vmState(v.ExpectedState),
		Reason:          v.Reason,
		NodeName:        v.NodeName,
		Stats:           newStats(v.Stats),
		URLs:            NewURLs(v, ingressDomain),
		CreatedAt:       v.CreatedAt,
		StartedAt:       when(v.StartedAt),
		UpdatedAt:       when(v.UpdatedAt),
		ManagedBy:       v.ManagedBy,
	}
}

// NewVMs presents a list of VMs.
func NewVMs(vms []vm.VM, ingressDomain string, owners Owners) []VM {
	items := make([]VM, len(vms))
	for i := range vms {
		items[i] = NewVM(vms[i], ingressDomain, owners)
	}

	return items
}

// NewURLs are where a VM's ports are served.
//
// Every port answers on the VM's slug with the port appended, which keeps
// each hostname a single label under the ingress domain, so one wildcard
// certificate covers them all. A VM whose ingress is denied is served
// nowhere, whatever ports it lists, and neither is one with no slug yet.
func NewURLs(v vm.VM, ingressDomain string) []URL {
	urls := make([]URL, 0, len(v.Ports))
	if v.Network.Ingress != vm.AccessAllow || len(v.Slug) == 0 || len(ingressDomain) == 0 {
		return urls
	}

	scheme := schemeOf(ingressDomain)
	for _, p := range v.Ports {
		urls = append(urls, URL{
			Port: uint(p),
			URL:  fmt.Sprintf("%s://%s-%d.%s", scheme, v.Slug, p, ingressDomain),
		})
	}

	return urls
}

// schemeOf is how the ingress is reached: over TLS, except under localhost,
// which is somebody's own machine, where the ingress has no certificate and
// every name resolves to the loopback address anyway.
func schemeOf(ingressDomain string) string {
	host := ingressDomain
	if h, _, err := net.SplitHostPort(ingressDomain); err == nil {
		host = h
	}

	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return "http"
	}

	return "https"
}

func newStats(s vm.Stats) *Stats {
	// a VM its node has not sampled yet has nothing to show, which is not the
	// same as using nothing.
	if s.SampledAt.IsZero() {
		return nil
	}

	return &Stats{
		CPUPercent:  s.CPUPercent,
		MemoryUsed:  s.MemoryUsed,
		MemoryLimit: s.MemoryLimit,
		DiskUsed:    s.DiskUsed,
		DiskTotal:   s.DiskTotal,
		NetworkRx:   s.NetworkRx,
		NetworkTx:   s.NetworkTx,
		SampledAt:   s.SampledAt,
	}
}

// vmState is a state's word, and nothing for one that was never set.
func vmState(s vm.State) string {
	if s == 0 {
		return ""
	}

	return s.String()
}

// when is a moment that may not have come yet: left out rather than shown as
// the first of January of year one.
func when(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}

	return &t
}
