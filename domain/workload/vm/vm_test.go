package vm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestKind_IsValid(t *testing.T) {
	t.Parallel()

	for kind, want := range map[Kind]bool{
		KindMachine:      true,
		KindDocker:       true,
		Kind(""):         false,
		Kind("Docker"):   false,
		Kind("firecore"): false,
	} {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, want, kind.IsValid())
		})
	}
}

func TestAccess_IsValid(t *testing.T) {
	t.Parallel()

	for access, want := range map[Access]bool{
		AccessAllow:     true,
		AccessDeny:      true,
		Access(""):      false,
		Access("ALLOW"): false,
		Access("open"):  false,
	} {
		t.Run(string(access), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, want, access.IsValid())
		})
	}
}

func TestVM_Expired(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

	for name, tt := range map[string]struct {
		vm   VM
		want bool
	}{
		"past the end of its lifetime": {
			vm:   VM{Lifetime: time.Hour, ExpiresAt: now.Add(-time.Second)},
			want: true,
		},
		"at the very moment it ends": {
			vm:   VM{Lifetime: time.Hour, ExpiresAt: now},
			want: true,
		},
		"still inside it": {
			vm:   VM{Lifetime: time.Hour, ExpiresAt: now.Add(time.Minute)},
			want: false,
		},
		"kept until it is deleted": {
			vm:   VM{},
			want: false,
		},
		"kept until it is deleted, whatever moment is written down": {
			vm:   VM{ExpiresAt: now.Add(-time.Hour)},
			want: false,
		},
		"given a lifetime that was never counted from anywhere": {
			vm:   VM{Lifetime: time.Hour},
			want: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.vm.Expired(now))
		})
	}
}

func TestVM_Silent(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	const after = 15 * time.Second

	for name, tt := range map[string]struct {
		vm   VM
		want bool
	}{
		"spoken for a moment ago": {
			vm:   VM{LastHeartbeatAt: now.Add(-time.Second)},
			want: false,
		},
		"not spoken for in a while": {
			vm:   VM{LastHeartbeatAt: now.Add(-time.Minute)},
			want: true,
		},
		"never spoken for, and asked for a while ago": {
			vm:   VM{CreatedAt: now.Add(-time.Minute)},
			want: true,
		},
		"never spoken for, but only just asked for": {
			vm:   VM{CreatedAt: now.Add(-time.Second)},
			want: false,
		},
		"nothing known about it at all": {
			vm:   VM{},
			want: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.vm.Silent(now, after))
		})
	}
}

func TestVM_Drifted(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	const silence = 15 * time.Second

	fresh := now.Add(-time.Second)
	quiet := now.Add(-time.Minute)

	for name, tt := range map[string]struct {
		vm   VM
		want bool
	}{
		"running as asked": {
			vm:   VM{ExpectedState: Running, CurrentState: Running, LastHeartbeatAt: fresh},
			want: false,
		},
		"stopped when it was meant to run": {
			vm:   VM{ExpectedState: Running, CurrentState: Stopped, LastHeartbeatAt: fresh},
			want: true,
		},
		"failed when it was meant to run": {
			vm:   VM{ExpectedState: Running, CurrentState: Failed, LastHeartbeatAt: fresh},
			want: true,
		},
		"running when it was meant to stop": {
			vm:   VM{ExpectedState: Stopped, CurrentState: Running, LastHeartbeatAt: fresh},
			want: true,
		},
		"failed when it was meant to stop, which it is": {
			vm:   VM{ExpectedState: Stopped, CurrentState: Failed, LastHeartbeatAt: fresh},
			want: false,
		},
		"on its way to stopping": {
			vm:   VM{ExpectedState: Stopped, CurrentState: Stopping, LastHeartbeatAt: fresh},
			want: false,
		},
		"being restored": {
			vm:   VM{ExpectedState: Running, CurrentState: Restoring, LastHeartbeatAt: fresh},
			want: false,
		},
		"on its way, but nobody has spoken for it in a while": {
			vm:   VM{ExpectedState: Stopped, CurrentState: Stopping, LastHeartbeatAt: quiet},
			want: true,
		},
		"last seen running, but gone quiet: it is not there any more": {
			vm:   VM{ExpectedState: Running, CurrentState: Running, LastHeartbeatAt: quiet},
			want: true,
		},
		"stopped as asked, and quiet about it": {
			vm:   VM{ExpectedState: Stopped, CurrentState: Stopped, LastHeartbeatAt: quiet},
			want: false,
		},
		"nothing was ever asked of it": {
			vm:   VM{CurrentState: Running, LastHeartbeatAt: quiet},
			want: false,
		},
		"never heard from at all, and still on its way": {
			vm:   VM{ExpectedState: Running, CurrentState: Scheduled},
			want: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.vm.Drifted(now, silence))
		})
	}
}
