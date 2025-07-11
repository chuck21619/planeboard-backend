package ws

type DiceRoller struct {
	ID       string  `json:"id"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	NumDice  int     `json:"numDice"`
	NumSides int     `json:"numSides"`
}
