package logs

import "time"

// Response is one line, as the dashboard shows it.
type Response struct {
	Stream  string    `json:"stream"`
	Content string    `json:"content"`
	At      time.Time `json:"at"`
}
