package ws

type Deck struct {
	ID    string  `json:"id"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Cards []Card  `json:"cards"`
}
