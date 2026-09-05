package followContainerLogs

import (
	"time"
)

// LineResponse is one line, as the dashboard shows it.
type LineResponse struct {
	Stream  string    `json:"stream"`
	Content string    `json:"content"`
	At      time.Time `json:"at"`
}
