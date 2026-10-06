// Package quota holds VMs to what one VM may be given and what one person's
// VMs may be given between them.
//
// Going past either is a request that cannot be taken as it stands, so both
// answer with validation errors, under the field that went past, as the
// dashboard names its fields: the person asking can see what to change.
package quota

import (
	"context"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/records"
	"github.com/khanzadimahdi/testproject/domain"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
)

// The codes a request is refused with.
const (
	// CodeTooSmall is less than one VM of its flavor may be given.
	CodeTooSmall = "too_small"

	// CodeTooLarge is more than one VM may be given.
	CodeTooLarge = "too_large"

	// CodeQuotaExceeded is more than one person may have between their VMs.
	CodeQuotaExceeded = "quota_exceeded"
)

// Limits are what one VM may be given, and what one person's VMs may be given
// between them. CPUs are whole vCPUs; memory and disk are bytes.
type Limits struct {
	// the least a VM may be given, which is more for a Docker VM: dockerd and
	// the images it pulls need room before anything runs in it.
	MinMemory       uint64
	MinDisk         uint64
	DockerMinMemory uint64
	DockerMinDisk   uint64

	// the most one VM may be given.
	MaxCPUs   uint
	MaxMemory uint64
	MaxDisk   uint64

	// the most one person may have, across all of their VMs.
	UserMaxVMs uint
	UserCPUs   uint
	UserMemory uint64
	UserDisk   uint64

	// MaxLifetime is the longest a VM may ask to be kept for.
	MaxLifetime time.Duration
}

// Bounds checks what one VM of a flavor is given against what one VM may be
// given. Each field is reported under prefix, so a VM asked for inside
// another request is reported where it was asked.
func (l Limits) Bounds(prefix string, flavor vmKind.Flavor, resources vmKind.Resources) domain.ValidationErrors {
	invalid := make(domain.ValidationErrors)

	minMemory, minDisk := l.MinMemory, l.MinDisk
	if flavor == vmKind.FlavorDocker {
		minMemory, minDisk = l.DockerMinMemory, l.DockerMinDisk
	}

	switch {
	case resources.CPUs == 0:
		invalid[prefix+"resources.cpus"] = "required_field"
	case resources.CPUs > l.MaxCPUs:
		invalid[prefix+"resources.cpus"] = CodeTooLarge
	}

	switch {
	case resources.Memory == 0:
		invalid[prefix+"resources.memory"] = "required_field"
	case resources.Memory < minMemory:
		invalid[prefix+"resources.memory"] = CodeTooSmall
	case resources.Memory > l.MaxMemory:
		invalid[prefix+"resources.memory"] = CodeTooLarge
	}

	switch {
	case resources.Disk == 0:
		invalid[prefix+"resources.disk"] = "required_field"
	case resources.Disk < minDisk:
		invalid[prefix+"resources.disk"] = CodeTooSmall
	case resources.Disk > l.MaxDisk:
		invalid[prefix+"resources.disk"] = CodeTooLarge
	}

	return invalid
}

// Lifetime checks how long a VM asks to be kept for. Zero is until it is
// deleted, which is always allowed.
func (l Limits) Lifetime(prefix string, lifetime time.Duration) domain.ValidationErrors {
	invalid := make(domain.ValidationErrors)

	switch {
	case lifetime < 0:
		invalid[prefix+"lifetime_seconds"] = "invalid_lifetime"
	case l.MaxLifetime > 0 && lifetime > l.MaxLifetime:
		invalid[prefix+"lifetime_seconds"] = CodeTooLarge
	}

	return invalid
}

// Quota holds people to what their VMs may be given between them.
type Quota struct {
	records *records.Records
	limits  Limits
}

func New(vms *records.Records, limits Limits) *Quota {
	return &Quota{records: vms, limits: limits}
}

// Limits are what the quota holds people to.
func (q *Quota) Limits() Limits {
	return q.limits
}

// Check reports what would take ownerUUID past their quota if one of their
// VMs were given resources: a new one when except is empty, or the one except
// names, in place of what it has now. A VM on its way out is not counted: it
// is going, and asking for its replacement is not asking for more.
func (q *Quota) Check(ctx context.Context, prefix string, ownerUUID string, resources vmKind.Resources, except string) (domain.ValidationErrors, error) {
	owned, err := q.records.Owned(ctx, ownerUUID)
	if err != nil {
		return nil, err
	}

	var (
		count uint
		total vmKind.Resources
	)

	for _, v := range owned {
		if v.Metadata.UUID == except || records.Going(v) {
			continue
		}

		count++
		total.CPUs += v.Spec.Resources.CPUs
		total.Memory += v.Spec.Resources.Memory
		total.Disk += v.Spec.Resources.Disk
	}

	invalid := make(domain.ValidationErrors)

	if len(except) == 0 && count+1 > q.limits.UserMaxVMs {
		invalid["vms"] = CodeQuotaExceeded
	}

	if total.CPUs+resources.CPUs > q.limits.UserCPUs {
		invalid[prefix+"resources.cpus"] = CodeQuotaExceeded
	}

	if total.Memory+resources.Memory > q.limits.UserMemory {
		invalid[prefix+"resources.memory"] = CodeQuotaExceeded
	}

	if total.Disk+resources.Disk > q.limits.UserDisk {
		invalid[prefix+"resources.disk"] = CodeQuotaExceeded
	}

	return invalid, nil
}
