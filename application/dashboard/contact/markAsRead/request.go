package markAsRead

type Request struct {
	MessageUUID string `json:"-"`
	// Read carries the toggle's new state: true stamps the read time, false
	// clears it back to unread.
	Read bool `json:"read"`
}
