package rules

import "testing"

func TestIsValidPhoneNumber(t *testing.T) {
	tests := []struct {
		phone string
		want  bool
	}{
		{"", false},
		{"1", false},
		{"123", false},
		{"1234", true},
		{"09123456789", true},
		{"12 34", false},
		{"+1234", false},
		{"12-34", false},
		{"12a4", false},
		{"۱۲۳۴", false},
	}

	for _, tt := range tests {
		t.Run(tt.phone, func(t *testing.T) {
			if got := IsValidPhoneNumber(tt.phone); got != tt.want {
				t.Errorf("IsValidPhoneNumber(%q) = %v, want %v", tt.phone, got, tt.want)
			}
		})
	}
}
