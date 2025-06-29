package ws

type BoardCard struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	ImageURL string  `json:"imageUrl"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Owner    string  `json:"owner"`
}

type Card struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ImageURL string `json:"imageUrl"`
}
