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
