package article

type Entity struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Summery string `json:"summery"`
	Body    string `json:"body"`
	Status  string `json:"status"`
}
