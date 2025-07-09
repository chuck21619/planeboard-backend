package ws

type Message struct {
	Type string `json:"type"`

	ID        string  `json:"id,omitempty"`
	X         float64 `json:"x,omitempty"`
	Y         float64 `json:"y,omitempty"`
	Tapped    bool    `json:"tapped,omitempty"`
	FlipIndex int     `json:"flipIndex,omitempty"`

	Username  string `json:"username,omitempty"`
	DeckURL   string `json:"deckUrl,omitempty"`
	LifeTotal *int   `json:"lifeTotal,omitempty"` // pointer so 0 is distinguishable from missing

	Cards []BoardCard `json:"cards,omitempty"`
	Card  BoardCard   `json:"card,omitempty"`
	Deck  Deck        `json:"deck,omitempty"`

	Source string `json:"source,omitempty"`

	Counters []Counter `json:"counters,omitempty"`
	Count    int       `json:"count,omitempty"`
}
