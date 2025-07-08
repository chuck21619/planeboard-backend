package ws

type Counter struct {
	ID    string  `json:"id"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Count int     `json:"count"`
	Owner string  `json:"owner"`
}
