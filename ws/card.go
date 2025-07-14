package ws

type Card struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ImageURL     string `json:"imageUrl"`
	ImageURLBack string `json:"imageUrlBack"`
	UID          string `json:"uid"`
	HasTokens    bool   `json:"hasTokens"`
	NumFaces     int    `json:"numFaces"`
	Token        bool   `json:"token"`
}

type BoardCard struct {
	Card
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Owner     string  `json:"owner"`
	Tapped    bool    `json:"tapped"`
	FlipIndex int     `json:"flipIndex"`
}
