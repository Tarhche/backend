package user

import "testing"

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
