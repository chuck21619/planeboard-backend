package ws

type Card struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ImageURL  string `json:"imageUrl"`
	UID       string `json:"uid"`
	HasTokens bool   `json:"hasTokens"`
}

type BoardCard struct {
	Card
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Owner  string  `json:"owner"`
	Tapped bool    `json:"tapped"`
}
