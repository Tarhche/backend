package heartbeat

type Response struct {
	Name string `json:"name"`
	Logs []byte `json:"logs"`
}
