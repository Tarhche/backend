package getusercontainerlogs

import (
	"time"
)

type Response struct {
	Items []LogResponse `json:"items"`
}

type LogResponse struct {
	Stream  string    `json:"stream"`
	Content string    `json:"content"`
	At      time.Time `json:"at"`
}
