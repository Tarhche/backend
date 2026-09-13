package getTaskLogs

import (
	"time"
)

type Response struct {
	Items []log `json:"items"`
}

type log struct {
	Stream  string    `json:"stream"`
	Content string    `json:"content"`
	At      time.Time `json:"at"`
}
