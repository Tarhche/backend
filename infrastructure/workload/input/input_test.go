package input

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// held runs a rule on a fresh set of errors and says what it found.
func held(rule func(errs domain.ValidationErrors)) domain.ValidationErrors {
	errs := make(domain.ValidationErrors)
	rule(errs)

	return errs
}

func TestName(t *testing.T) {
	t.Parallel()

	assert.Empty(t, held(func(e domain.ValidationErrors) { Name(e, "name", "My web box ✓") }), "a name is anything somebody likes")
	assert.Equal(t, domain.ValidationErrors{"name": "required_field"}, held(func(e domain.ValidationErrors) { Name(e, "name", "") }))
	assert.Equal(t, domain.ValidationErrors{"name": "required_field"}, held(func(e domain.ValidationErrors) { Name(e, "name", "   ") }), "a blank one is none")
	assert.Equal(t, domain.ValidationErrors{"name": "exceeds_limit"}, held(func(e domain.ValidationErrors) { Name(e, "name", strings.Repeat("ب", MaxNameLength+1)) }))
	assert.Empty(t, held(func(e domain.ValidationErrors) { Name(e, "name", strings.Repeat("ب", MaxNameLength)) }), "counted in characters, not bytes")
}

func TestDockerName(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"", "web", "web-1", "my_app.v2", "9lives"} {
		assert.Empty(t, held(func(e domain.ValidationErrors) { DockerName(e, "name", name) }), name)
	}

	for _, name := range []string{"a", "-web", ".hidden", "web app", "web/1", "ünicode"} {
		assert.Equal(t, domain.ValidationErrors{"name": "invalid_name"}, held(func(e domain.ValidationErrors) { DockerName(e, "name", name) }), name)
	}
}

func TestImage(t *testing.T) {
	t.Parallel()

	for _, reference := range []string{"nginx", "nginx:1.27", "ghcr.io/acme/app@sha256:abc", "localhost:5000/app:latest"} {
		assert.Empty(t, held(func(e domain.ValidationErrors) { Image(e, "image", reference) }), reference)
	}

	assert.Equal(t, domain.ValidationErrors{"image": "required_field"}, held(func(e domain.ValidationErrors) { Image(e, "image", " ") }))
	assert.Equal(t, domain.ValidationErrors{"image": "invalid_image"}, held(func(e domain.ValidationErrors) { Image(e, "image", "nginx latest") }))
}

func TestPorts(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name  string
		ports []uint
		want  domain.ValidationErrors
	}{
		{name: "none is fine", ports: nil, want: domain.ValidationErrors{}},
		{name: "ports from 1 to 65535", ports: []uint{1, 80, 65535}, want: domain.ValidationErrors{}},
		{name: "a port that is no port is named by where it is", ports: []uint{80, 0, 70000}, want: domain.ValidationErrors{"ports.1": "invalid_port", "ports.2": "invalid_port"}},
		{name: "the same port twice is named where it comes again", ports: []uint{80, 443, 80}, want: domain.ValidationErrors{"ports.2": "duplicate_port"}},
		{name: "at most 16", ports: []uint{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17}, want: domain.ValidationErrors{"ports": "too_many_ports"}},
		{name: "16 is not too many", ports: []uint{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, want: domain.ValidationErrors{}},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, held(func(e domain.ValidationErrors) { Ports(e, "ports", tt.ports) }))
		})
	}
}

func TestLifetime(t *testing.T) {
	t.Parallel()

	assert.Empty(t, held(func(e domain.ValidationErrors) { Lifetime(e, "lifetime_seconds", 0) }), "zero keeps it until it is deleted")
	assert.Empty(t, held(func(e domain.ValidationErrors) { Lifetime(e, "lifetime_seconds", 3600) }))
	assert.Equal(t, domain.ValidationErrors{"lifetime_seconds": "invalid_lifetime"}, held(func(e domain.ValidationErrors) { Lifetime(e, "lifetime_seconds", -1) }))
	assert.Equal(t, domain.ValidationErrors{"lifetime_seconds": "invalid_lifetime"}, held(func(e domain.ValidationErrors) { Lifetime(e, "lifetime_seconds", math.MaxInt64) }), "nothing that would wrap round")

	assert.Equal(t, int64(3600), int64(LifetimeOf(3600).Seconds()))
}

func TestPortsOf(t *testing.T) {
	t.Parallel()

	assert.Nil(t, PortsOf(nil))
	assert.Equal(t, []port.Port{22, 80, 443}, PortsOf([]uint{443, 22, 80}), "a VM keeps its ports in order")
}

func TestResources(t *testing.T) {
	t.Parallel()

	whole := Resources{CPUs: 1, Memory: 1 << 30, Disk: 10 << 30}

	assert.Empty(t, held(func(e domain.ValidationErrors) { whole.Validate(e, "resources", false) }))
	assert.Equal(t, domain.ValidationErrors{"resources": "required_field"}, held(func(e domain.ValidationErrors) { Resources{}.Validate(e, "resources", false) }))
	assert.Equal(t,
		domain.ValidationErrors{"resources.cpus": "greater_than_zero", "resources.disk": "greater_than_zero"},
		held(func(e domain.ValidationErrors) { Resources{Memory: 1 << 30}.Validate(e, "resources", false) }),
	)
	assert.Empty(t, held(func(e domain.ValidationErrors) { Resources{CPUs: 1, Memory: 1 << 30}.Validate(e, "resources", true) }), "a snapshot may say how big the disk is")

	assert.Equal(t, vm.Resources{CPUs: 1, Memory: 1 << 30, Disk: 10 << 30}, whole.VM())
}

func TestNetwork(t *testing.T) {
	t.Parallel()

	assert.Empty(t, held(func(e domain.ValidationErrors) { Network{Ingress: "allow", Egress: "deny"}.Validate(e, "network") }))
	assert.Equal(t, domain.ValidationErrors{"network": "required_field"}, held(func(e domain.ValidationErrors) { Network{}.Validate(e, "network") }))
	assert.Equal(t,
		domain.ValidationErrors{"network.ingress": "required_field", "network.egress": "invalid_access"},
		held(func(e domain.ValidationErrors) { Network{Egress: "public"}.Validate(e, "network") }),
	)

	// a new Docker VM's network may say only one way, and the default fills in
	// the other.
	assert.Empty(t, held(func(e domain.ValidationErrors) { Network{Ingress: "deny"}.ValidatePartly(e, "vm.network") }))
	assert.Equal(t,
		domain.ValidationErrors{"vm.network.egress": "invalid_access"},
		held(func(e domain.ValidationErrors) { Network{Egress: "open"}.ValidatePartly(e, "vm.network") }),
	)

	assert.Equal(t, vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny}, Network{Ingress: "allow", Egress: "deny"}.VM())
}

func TestDockerVM(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name      string
		vmUUID    string
		described *NewDockerVM
		want      domain.ValidationErrors
	}{
		{name: "neither is a new one with the defaults", want: domain.ValidationErrors{}},
		{name: "one named", vmUUID: "vm-uuid", want: domain.ValidationErrors{}},
		{name: "a new one, all defaults", described: &NewDockerVM{}, want: domain.ValidationErrors{}},
		{name: "naming one and describing another is asking for two things", vmUUID: "vm-uuid", described: &NewDockerVM{}, want: domain.ValidationErrors{"vm": "vm_or_new_vm"}},
		{
			name:      "a new one is held to what a VM is",
			described: &NewDockerVM{Name: " ", Ports: []uint{80, 80}, Network: &Network{Ingress: "maybe"}},
			want:      domain.ValidationErrors{"vm.name": "required_field", "vm.ports.1": "duplicate_port", "vm.network.ingress": "invalid_access"},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, held(func(e domain.ValidationErrors) { DockerVM(e, tt.vmUUID, tt.described) }))
		})
	}
}

func TestDockerVMChoice(t *testing.T) {
	t.Parallel()

	assert.Equal(t, workloadControlPlane.DockerVMChoice{UUID: "vm-uuid"}, DockerVMChoice("vm-uuid", nil))
	assert.Equal(t, workloadControlPlane.DockerVMChoice{}, DockerVMChoice("", nil), "neither is a new one with the workload's defaults")

	assert.Equal(t,
		workloadControlPlane.DockerVMChoice{New: &workloadControlPlane.NewDockerVM{
			Name:      "docker-1",
			Resources: &vm.Resources{CPUs: 4},
			Ports:     []port.Port{80, 443},
			Network:   &vm.Network{Ingress: vm.AccessAllow},
		}},
		DockerVMChoice("", &NewDockerVM{
			Name:      "docker-1",
			Resources: &Resources{CPUs: 4},
			Ports:     []uint{443, 80},
			Network:   &Network{Ingress: "allow"},
		}),
		"what is left empty is the workload's default",
	)
}
