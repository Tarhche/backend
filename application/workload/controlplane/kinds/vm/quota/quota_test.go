package quota_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/quota"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func TestLimits_Bounds(t *testing.T) {
	t.Parallel()

	limits := vmtest.Limits

	for name, tt := range map[string]struct {
		kind      vm.Kind
		resources vmKind.Resources
		want      domain.ValidationErrors
	}{
		"a machine within its bounds": {
			kind:      vm.KindMachine,
			resources: vmKind.Resources{CPUs: 4, Memory: 128 * vmtest.MiB, Disk: 50 * vmtest.GiB},
			want:      domain.ValidationErrors{},
		},
		"more than one vm may be given": {
			kind:      vm.KindMachine,
			resources: vmKind.Resources{CPUs: 5, Memory: 8*vmtest.GiB + 1, Disk: 50*vmtest.GiB + 1},
			want:      domain.ValidationErrors{"resources.cpus": "too_large", "resources.memory": "too_large", "resources.disk": "too_large"},
		},
		"less than a machine needs": {
			kind:      vm.KindMachine,
			resources: vmKind.Resources{CPUs: 1, Memory: 127 * vmtest.MiB, Disk: vmtest.GiB - 1},
			want:      domain.ValidationErrors{"resources.memory": "too_small", "resources.disk": "too_small"},
		},
		"what a machine gets by on is too little for a docker vm": {
			kind:      vm.KindDocker,
			resources: vmKind.Resources{CPUs: 1, Memory: 256 * vmtest.MiB, Disk: 2 * vmtest.GiB},
			want:      domain.ValidationErrors{"resources.memory": "too_small", "resources.disk": "too_small"},
		},
		"nothing at all": {
			kind: vm.KindMachine,
			want: domain.ValidationErrors{"resources.cpus": "required_field", "resources.memory": "required_field", "resources.disk": "required_field"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, limits.Bounds("", tt.kind, tt.resources))
		})
	}

	t.Run("each field is said where it was asked", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, domain.ValidationErrors{"vm.new.resources.cpus": "too_large"}, limits.Bounds("vm.new.", vm.KindDocker, vmKind.Resources{CPUs: 8, Memory: vmtest.GiB, Disk: 10 * vmtest.GiB}))
	})
}

func TestLimits_Lifetime(t *testing.T) {
	t.Parallel()

	limits := vmtest.Limits

	assert.Empty(t, limits.Lifetime("", 0), "kept until it is deleted")
	assert.Empty(t, limits.Lifetime("", 720*time.Hour))
	assert.Equal(t, domain.ValidationErrors{"lifetime_seconds": "too_large"}, limits.Lifetime("", 721*time.Hour))
	assert.Equal(t, domain.ValidationErrors{"lifetime_seconds": "invalid_lifetime"}, limits.Lifetime("", -time.Second))
	assert.Equal(t, domain.ValidationErrors{"vm.new.lifetime_seconds": "too_large"}, limits.Lifetime("vm.new.", 721*time.Hour))

	limits.MaxLifetime = 0
	assert.Empty(t, limits.Lifetime("", 10000*time.Hour), "no longest is any length")
}

func TestQuota_Check(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	sized := func(uuid string, ownerUUID string, resources vmKind.Resources) vmKind.VM {
		return vmtest.In(vmtest.Running(uuid, ownerUUID), func(v *vmKind.VM) { v.Spec.Resources = resources })
	}

	w := vmtest.New(vmtest.WithVMs(
		sized("01", "owner", vmKind.Resources{CPUs: 4, Memory: 8 * vmtest.GiB, Disk: 100 * vmtest.GiB}),
		sized("02", "owner", vmKind.Resources{CPUs: 2, Memory: 4 * vmtest.GiB, Disk: 50 * vmtest.GiB}),
		vmtest.In(sized("03", "owner", vmKind.Resources{CPUs: 4, Memory: 8 * vmtest.GiB, Disk: 50 * vmtest.GiB}), func(v *vmKind.VM) {
			v.Status.Expected = vmKind.Deleted
		}),
		vmtest.In(sized("04", "owner", vmKind.Resources{CPUs: 4, Memory: 8 * vmtest.GiB, Disk: 50 * vmtest.GiB}), func(v *vmKind.VM) {
			v.Status.State = vmKind.Deleting
		}),
		sized("05", "other", vmKind.Resources{CPUs: 4, Memory: 8 * vmtest.GiB, Disk: 50 * vmtest.GiB}),
	))

	for name, tt := range map[string]struct {
		ownerUUID string
		resources vmKind.Resources
		except    string
		prefix    string
		want      domain.ValidationErrors
	}{
		"a new one within what is left": {
			ownerUUID: "owner",
			resources: vmKind.Resources{CPUs: 2, Memory: 4 * vmtest.GiB, Disk: 50 * vmtest.GiB},
			want:      domain.ValidationErrors{},
		},
		"a new one past what is left, those on their way out not counted": {
			ownerUUID: "owner",
			resources: vmKind.Resources{CPUs: 3, Memory: 4*vmtest.GiB + 1, Disk: 50*vmtest.GiB + 1},
			want:      domain.ValidationErrors{"resources.cpus": "quota_exceeded", "resources.memory": "quota_exceeded", "resources.disk": "quota_exceeded"},
		},
		"one of theirs grown into what is left, counted in place of what it has": {
			ownerUUID: "owner",
			resources: vmKind.Resources{CPUs: 6, Memory: 12 * vmtest.GiB, Disk: 150 * vmtest.GiB},
			except:    "01",
			want:      domain.ValidationErrors{},
		},
		"and past it": {
			ownerUUID: "owner",
			resources: vmKind.Resources{CPUs: 7, Memory: 12 * vmtest.GiB, Disk: 150 * vmtest.GiB},
			except:    "01",
			prefix:    "vm.new.",
			want:      domain.ValidationErrors{"vm.new.resources.cpus": "quota_exceeded"},
		},
		"somebody else's are theirs alone": {
			ownerUUID: "newcomer",
			resources: vmKind.Resources{CPUs: 8, Memory: 16 * vmtest.GiB, Disk: 200 * vmtest.GiB},
			want:      domain.ValidationErrors{},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			invalid, err := w.Quota.Check(ctx, tt.prefix, tt.ownerUUID, tt.resources, tt.except)
			require.NoError(t, err)
			assert.Equal(t, tt.want, invalid)
		})
	}

	t.Run("more vms than one person may have", func(t *testing.T) {
		t.Parallel()

		many := make([]vmKind.VM, vmtest.Limits.UserMaxVMs)
		for i := range many {
			many[i] = sized(string(rune('a'+i)), "owner", vmKind.Resources{CPUs: 1, Memory: vmtest.GiB, Disk: vmtest.GiB})
		}

		full := vmtest.New(vmtest.WithVMs(many...))

		invalid, err := full.Quota.Check(ctx, "", "owner", vmKind.Resources{CPUs: 1, Memory: vmtest.GiB, Disk: vmtest.GiB}, "")
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vms": "quota_exceeded"}, invalid)

		invalid, err = full.Quota.Check(ctx, "", "owner", vmKind.Resources{CPUs: 1, Memory: vmtest.GiB, Disk: vmtest.GiB}, "a")
		require.NoError(t, err)
		assert.Empty(t, invalid, "one of them changing is not one more")
	})

	t.Run("what it holds people to", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, vmtest.Limits, quota.New(w.Records, vmtest.Limits).Limits())
	})
}
