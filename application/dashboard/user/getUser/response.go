package getuser

import "time"

type Response struct {
	UUID         string `json:"uuid,omitempty"`
	Name         string `json:"name,omitempty"`
	Avatar       string `json:"avatar,omitempty"`
	Email        string `json:"email,omitempty"`
	Username     string `json:"username,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
	// Not omitempty: "not banned" is a meaningful false, not an absent value.
	// BannedAt rides along so the dashboard can show since when; it is the zero
	// time for an account that was never banned.
	Banned   bool      `json:"banned"`
	BannedAt time.Time `json:"banned_at"`
}
