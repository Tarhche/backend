package user

import (
	"testing"
	"time"
)

func TestIsBanned(t *testing.T) {
	tests := []struct {
		name     string
		bannedAt time.Time
		want     bool
	}{
		{"never banned", time.Time{}, false},
		{"banned in the past", time.Now().Add(-time.Hour), true},
		{"banned from now", time.Now().Add(-time.Second), true},
		{"ban starts later", time.Now().Add(time.Hour), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (User{BannedAt: tt.bannedAt}).IsBanned(); got != tt.want {
				t.Errorf("IsBanned() = %v, want %v", got, tt.want)
			}
		})
	}
}
