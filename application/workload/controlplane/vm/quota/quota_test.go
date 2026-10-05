package quota

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	vmsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/vms"
)

const (
	mib = 1 << 20
	gib = 1 << 30
)

var limits = Limits{
	MinMemory:       128 * mib,
	MinDisk:         1 * gib,
	DockerMinMemory: 512 * mib,
	DockerMinDisk:   4 * gib,
	MaxCPUs:         4,
	MaxMemory:       8 * gib,
	MaxDisk:         50 * gib,
	UserMaxVMs:      2,
	UserCPUs:        6,
	UserMemory:      10 * gib,
	UserDisk:        60 * gib,
	MaxLifetime:     720 * time.Hour,
}

func TestLimits_Bounds(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		kind      vm.Kind
		resources vm.Resources
		want      domain.ValidationErrors
	}{
		"within what one vm may be given": {
			kind:      vm.KindMachine,
			resources: vm.Resources{CPUs: 1, Memory: 128 * mib, Disk: gib},
			want:      domain.ValidationErrors{},
		},
		"at the most one vm may be given": {
			kind:      vm.KindMachine,
			resources: vm.Resources{CPUs: 4, Memory: 8 * gib, Disk: 50 * gib},
			want:      domain.ValidationErrors{},
		},
		"nothing asked for at all": {
			kind:      vm.KindMachine,
			resources: vm.Resources{},
			want: domain.ValidationErrors{
				"resources.cpus":   "required_field",
				"resources.memory": "required_field",
				"resources.disk":   "required_field",
			},
		},
		"less than a machine needs": {
			kind:      vm.KindMachine,
			resources: vm.Resources{CPUs: 1, Memory: 128*mib - 1, Disk: gib - 1},
			want: domain.ValidationErrors{
				"resources.memory": CodeTooSmall,
				"resources.disk":   CodeTooSmall,
			},
		},
		"enough for a machine is not enough for a docker vm": {
			kind:      vm.KindDocker,
			resources: vm.Resources{CPUs: 1, Memory: 256 * mib, Disk: 2 * gib},
			want: domain.ValidationErrors{
				"resources.memory": CodeTooSmall,
				"resources.disk":   CodeTooSmall,
			},
		},
		"more than one vm may be given": {
			kind:      vm.KindMachine,
			resources: vm.Resources{CPUs: 5, Memory: 8*gib + 1, Disk: 50*gib + 1},
			want: domain.ValidationErrors{
				"resources.cpus":   CodeTooLarge,
				"resources.memory": CodeTooLarge,
				"resources.disk":   CodeTooLarge,
			},
		},
		// memory is bytes, so 512 meant as mebibytes is 512 bytes.
		"memory written as if it were mebibytes": {
			kind:      vm.KindMachine,
			resources: vm.Resources{CPUs: 1, Memory: 512, Disk: gib},
			want:      domain.ValidationErrors{"resources.memory": CodeTooSmall},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, limits.Bounds("", tt.kind, tt.resources))
		})
	}

	t.Run("a vm asked for inside another request is reported where it was asked", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t,
			domain.ValidationErrors{"vm.new.resources.cpus": CodeTooLarge},
			limits.Bounds("vm.new.", vm.KindDocker, vm.Resources{CPUs: 8, Memory: gib, Disk: 10 * gib}),
		)
	})
}

func TestLimits_Lifetime(t *testing.T) {
	t.Parallel()

	assert.Empty(t, limits.Lifetime("", 0), "kept until it is deleted")
	assert.Empty(t, limits.Lifetime("", 720*time.Hour))
	assert.Equal(t, domain.ValidationErrors{"lifetime_seconds": CodeTooLarge}, limits.Lifetime("", 721*time.Hour))
	assert.Equal(t, domain.ValidationErrors{"lifetime_seconds": "invalid_lifetime"}, limits.Lifetime("", -time.Second))
}

func TestQuota_Check(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	one := vm.Resources{CPUs: 2, Memory: 4 * gib, Disk: 20 * gib}

	for name, tt := range map[string]struct {
		owned     []vm.VM
		resources vm.Resources
		except    string
		want      domain.ValidationErrors
	}{
		"room for another": {
			owned:     []vm.VM{{UUID: "01", OwnerUUID: "owner", Resources: one}},
			resources: one,
			want:      domain.ValidationErrors{},
		},
		"too many vms": {
			owned: []vm.VM{
				{UUID: "01", OwnerUUID: "owner", Resources: vm.Resources{CPUs: 1, Memory: gib, Disk: gib}},
				{UUID: "02", OwnerUUID: "owner", Resources: vm.Resources{CPUs: 1, Memory: gib, Disk: gib}},
			},
			resources: vm.Resources{CPUs: 1, Memory: gib, Disk: gib},
			want:      domain.ValidationErrors{"vms": CodeQuotaExceeded},
		},
		"more than one person's vms may be given between them": {
			owned:     []vm.VM{{UUID: "01", OwnerUUID: "owner", Resources: vm.Resources{CPUs: 4, Memory: 8 * gib, Disk: 50 * gib}}},
			resources: vm.Resources{CPUs: 3, Memory: 3 * gib, Disk: 11 * gib},
			want: domain.ValidationErrors{
				"resources.cpus":   CodeQuotaExceeded,
				"resources.memory": CodeQuotaExceeded,
				"resources.disk":   CodeQuotaExceeded,
			},
		},
		"somebody else's vms are theirs": {
			owned: []vm.VM{
				{UUID: "01", OwnerUUID: "other", Resources: vm.Resources{CPUs: 4, Memory: 8 * gib, Disk: 50 * gib}},
				{UUID: "02", OwnerUUID: "other", Resources: vm.Resources{CPUs: 4, Memory: 8 * gib, Disk: 50 * gib}},
			},
			resources: one,
			want:      domain.ValidationErrors{},
		},
		"a vm on its way out is not counted": {
			owned: []vm.VM{
				{UUID: "01", OwnerUUID: "owner", Resources: one, CurrentState: vm.Deleting},
				{UUID: "02", OwnerUUID: "owner", Resources: one},
			},
			resources: one,
			want:      domain.ValidationErrors{},
		},
		"a vm that grows is counted as what it will have": {
			owned: []vm.VM{
				{UUID: "01", OwnerUUID: "owner", Resources: one},
				{UUID: "02", OwnerUUID: "owner", Resources: one},
			},
			resources: vm.Resources{CPUs: 4, Memory: 6 * gib, Disk: 40 * gib},
			except:    "02",
			want:      domain.ValidationErrors{},
		},
		"and is held to the quota too": {
			owned: []vm.VM{
				{UUID: "01", OwnerUUID: "owner", Resources: one},
				{UUID: "02", OwnerUUID: "owner", Resources: one},
			},
			resources: vm.Resources{CPUs: 4, Memory: 7 * gib, Disk: 40 * gib},
			except:    "02",
			want:      domain.ValidationErrors{"resources.memory": CodeQuotaExceeded},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := New(vmsMemory.NewRepository(tt.owned...), limits).Check(ctx, "", "owner", tt.resources, tt.except)
			require.NoError(t, err)

			assert.Equal(t, tt.want, got)
		})
	}
}
