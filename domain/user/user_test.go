package user

import (
	"testing"
	"time"
)

func TestIsValidEmail(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  bool
	}{
		{"plain", "user@example.com", true},
		{"with dots", "first.last@example.com", true},
		{"with plus tag", "user+tag@example.com", true},
		{"with underscore", "first_last@example.com", true},
		{"subdomain", "user@mail.example.com", true},
		{"with percent", "user%test@example.com", true},

		{"empty", "", false},
		{"missing at", "userexample.com", false},
		{"missing tld", "user@example", false},
		{"tld too short", "user@example.c", false},
		{"tld too long", "user@example.companyy", false},
		{"trailing space", "user@example.com ", false},
		{"uppercase", "User@Example.com", false},
		{"missing local part", "@example.com", false},
		{"missing domain", "user@", false},
		{"contains space", "us er@example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidEmail(tt.email); got != tt.want {
				t.Errorf("IsValidEmail(%q) = %v, want %v", tt.email, got, tt.want)
			}
		})
	}
}

func TestIsValidUsername(t *testing.T) {
	tests := []struct {
		name     string
		username string
		want     bool
	}{
		{"plain lowercase", "johndoe", true},
		{"digits only", "12345", true},
		{"letters and digits", "user123", true},
		{"with dot", "john.doe", true},
		{"with dash", "john-doe", true},
		{"with underscore", "john_doe", true},
		{"mixed allowed punctuation", "j.o-h_n.1", true},

		{"empty", "", false},
		{"only dots", "...", false},
		{"only dashes", "---", false},
		{"only underscores", "___", false},
		{"only punctuation", ".-_.", false},
		{"uppercase letter", "JohnDoe", false},
		{"space", "john doe", false},
		{"plus", "john+doe", false},
		{"at sign", "john@doe", false},
		{"non-ascii", "johnë", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidUsername(tt.username); got != tt.want {
				t.Errorf("IsValidUsername(%q) = %v, want %v", tt.username, got, tt.want)
			}
		})
	}
}

func TestSetBanned(t *testing.T) {
	earlier := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)

	t.Run("banning marks the moment", func(t *testing.T) {
		u := User{}
		u.SetBanned(true)

		if !u.IsBanned() {
			t.Fatal("expected the user to be banned")
		}
		if u.BannedAt.IsZero() {
			t.Error("expected BannedAt to be set")
		}
	})

	t.Run("banning again keeps the original moment", func(t *testing.T) {
		u := User{BannedAt: earlier}
		u.SetBanned(true)

		if !u.BannedAt.Equal(earlier) {
			t.Errorf("expected BannedAt to stay %v, got %v", earlier, u.BannedAt)
		}
	})

	t.Run("lifting the ban clears the moment", func(t *testing.T) {
		u := User{BannedAt: earlier}
		u.SetBanned(false)

		if u.IsBanned() {
			t.Fatal("expected the user not to be banned")
		}
		if !u.BannedAt.IsZero() {
			t.Errorf("expected BannedAt to be cleared, got %v", u.BannedAt)
		}
	})

	t.Run("a new user is not banned", func(t *testing.T) {
		if (User{}).IsBanned() {
			t.Error("expected a new user not to be banned")
		}
	})
}
