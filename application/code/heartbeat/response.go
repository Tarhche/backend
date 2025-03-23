package heartbeat

type Response struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	Logs []byte `json:"logs"`
}
