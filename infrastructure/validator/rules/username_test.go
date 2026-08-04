package rules

import "testing"

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
