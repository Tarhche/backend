package createVM

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost"
	"github.com/khanzadimahdi/testproject/application/workload/vmhost/vmhosttest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func accepts() *validator.MockValidator {
	v := &validator.MockValidator{}
	v.On("Validate", mock.Anything).Return(domain.ValidationErrors{})

	return v
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	limits := Limits{MaxMemory: 2 << 30, MaxCPU: 2}

	t.Run("a vm is made, and booted by nothing", func(t *testing.T) {
		t.Parallel()

		host := vmhosttest.New(t, nil)

		response, err := NewUseCase(host.Engine, accepts(), limits).Execute(ctx, &Request{Spec: vmhosttest.Spec("web")})
		require.NoError(t, err)

		assert.Empty(t, response.ValidationErrors)
		assert.True(t, vm.IsID(response.ID))
		assert.Equal(t, vm.StateCreated, host.VM(t, response.ID).State)
		assert.Empty(t, host.Hypervisor.Booted())
	})

	t.Run("more than one vm may have here is invalid, and nothing is made", func(t *testing.T) {
		t.Parallel()

		host := vmhosttest.New(t, nil)

		spec := vmhosttest.Spec("huge")
		spec.Resources.Memory = 4 << 30
		spec.Resources.CPU = 3

		response, err := NewUseCase(host.Engine, accepts(), limits).Execute(ctx, &Request{Spec: spec})
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{"resources.memory": "exceeds_limit", "resources.cpu": "exceeds_limit"}, response.ValidationErrors)

		vms, err := host.Engine.VMs(ctx, nil)
		require.NoError(t, err)
		assert.Empty(t, vms)
	})

	t.Run("no room left is the engine's refusal", func(t *testing.T) {
		t.Parallel()

		host := vmhosttest.New(t, nil, func(c *vmhost.Config) { c.MaxMemory = 320 << 20 })
		useCase := NewUseCase(host.Engine, accepts(), limits)

		_, err := useCase.Execute(ctx, &Request{Spec: vmhosttest.Spec("first")})
		require.NoError(t, err)

		_, err = useCase.Execute(ctx, &Request{Spec: vmhosttest.Spec("second")})
		assert.ErrorIs(t, err, vm.ErrCapacity)
	})

	t.Run("a name another vm answers to is a conflict", func(t *testing.T) {
		t.Parallel()

		host := vmhosttest.New(t, nil)
		useCase := NewUseCase(host.Engine, accepts(), limits)

		_, err := useCase.Execute(ctx, &Request{Spec: vmhosttest.Spec("web")})
		require.NoError(t, err)

		_, err = useCase.Execute(ctx, &Request{Spec: vmhosttest.Spec("web")})
		assert.ErrorIs(t, err, vm.ErrConflict)
	})

	t.Run("what is not valid is said so", func(t *testing.T) {
		t.Parallel()

		host := vmhosttest.New(t, nil)

		refuses := &validator.MockValidator{}
		refuses.On("Validate", mock.Anything).Return(domain.ValidationErrors{"name": "required_field"})

		response, err := NewUseCase(host.Engine, refuses, limits).Execute(ctx, &Request{})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"name": "required_field"}, response.ValidationErrors)
	})
}

func TestRequest_Validate(t *testing.T) {
	t.Parallel()

	t.Run("a vm as the orchestrator asks for one is valid", func(t *testing.T) {
		t.Parallel()

		spec := vmhosttest.Spec("nginx-xkfqz")
		spec.Env = []string{"PATH=/usr/bin", "EMPTY=", "INHERITED"}
		spec.WorkingDir = "/srv"
		spec.RestartPolicy = "on-failure:3"
		spec.Networks = []vm.Attachment{
			{Network: "workload-stack-shop", Aliases: []string{"web"}},
			{Network: vm.PublicNetwork, Gateway: true},
		}

		assert.Empty(t, (&Request{Spec: spec}).Validate())
	})

	t.Run("a vm with nothing to run as is refused", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, domain.ValidationErrors{
			"name":  "required_field",
			"image": "required_field",
		}, (&Request{}).Validate())
	})

	t.Run("what could reach where it should not is refused", func(t *testing.T) {
		t.Parallel()

		spec := vmhosttest.Spec("web")
		spec.Name = "../web"
		spec.Hostname = "web.example.com"
		spec.Image = "nginx alpine"
		spec.Labels = map[string]string{"a=b": "c"}
		spec.Env = []string{"=value"}
		spec.WorkingDir = "srv"
		spec.Resources.CPU = -1
		spec.RestartPolicy = "sometimes"
		spec.Networks = []vm.Attachment{{Network: "docker0/../host"}}
		spec.ExposedPorts = []uint16{0}

		assert.Equal(t, domain.ValidationErrors{
			"name":           "invalid_value",
			"hostname":       "invalid_value",
			"image":          "invalid_value",
			"labels":         "invalid_value",
			"env":            "invalid_value",
			"working_dir":    "invalid_value",
			"resources.cpu":  "invalid_value",
			"restart_policy": "invalid_value",
			"networks":       "invalid_value",
			"exposed_ports":  "invalid_value",
		}, (&Request{Spec: spec}).Validate())
	})

	t.Run("networks are joined once each, at most four, routing out through one", func(t *testing.T) {
		t.Parallel()

		twice := vmhosttest.Spec("web")
		twice.Networks = []vm.Attachment{{Network: "workload-isolated"}, {Network: "workload-isolated"}}
		assert.Equal(t, "invalid_value", (&Request{Spec: twice}).Validate()["networks"])

		gateways := vmhosttest.Spec("web")
		gateways.Networks = []vm.Attachment{{Network: "a", Gateway: true}, {Network: "b", Gateway: true}}
		assert.Equal(t, "invalid_value", (&Request{Spec: gateways}).Validate()["networks"])

		many := vmhosttest.Spec("web")
		many.Networks = []vm.Attachment{{Network: "a"}, {Network: "b"}, {Network: "c"}, {Network: "d"}, {Network: "e"}}
		assert.Equal(t, "exceeds_limit", (&Request{Spec: many}).Validate()["networks"])

		alias := vmhosttest.Spec("web")
		alias.Networks = []vm.Attachment{{Network: "a", Aliases: []string{"web; rm -rf /"}}}
		assert.Equal(t, "invalid_value", (&Request{Spec: alias}).Validate()["networks"])
	})
}
