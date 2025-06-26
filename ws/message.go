package ws

type Message struct {
	Type string `json:"type"`

	ID string  `json:"id,omitempty"`
	X  float64 `json:"x,omitempty"`
	Y  float64 `json:"y,omitempty"`

	Username string `json:"username,omitempty"`
	DeckURL  string `json:"deckUrl,omitempty"`

	Cards []BoardCard `json:"cards,omitempty"`
}
