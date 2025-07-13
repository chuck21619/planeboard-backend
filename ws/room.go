package ws

import (
	"encoding/json"
	"log"
	"sync"
)

var defaultPositions = []string{"bottomLeft", "topLeft", "topRight", "bottomRight"}

type Room struct {
	ID              string
	Clients         map[*Client]bool
	Spectators      map[*Client]bool
	Register        chan *Client
	Unregister      chan *Client
	Broadcast       chan []byte
	Cards           map[string]*BoardCard
	mu              sync.Mutex
	DeckURLs        map[string]string
	Decks           map[string]*Deck
	PlayerPositions map[string]string
	HandSizes       map[string]int
	LifeTotals      map[string]int
	Turn            string
	Counters        map[string]*Counter
	DiceRollers     map[string]*DiceRoller
}

func NewRoom(id string) *Room {
	return &Room{
		ID:              id,
		Clients:         make(map[*Client]bool),
		Spectators:      make(map[*Client]bool),
		Register:        make(chan *Client),
		Unregister:      make(chan *Client),
		Broadcast:       make(chan []byte),
		Cards:           make(map[string]*BoardCard),
		DeckURLs:        make(map[string]string),
		Decks:           make(map[string]*Deck),
		PlayerPositions: make(map[string]string),
		HandSizes:       make(map[string]int),
		LifeTotals:      make(map[string]int),
		Turn:            "",
		Counters:        make(map[string]*Counter),
		DiceRollers:     make(map[string]*DiceRoller),
	}
}

func (r *Room) BroadcastSafe(msg []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for client := range r.Clients {
		select {
		case client.Send <- msg:
			// message sent successfully
		default:
			log.Printf("dropping unresponsive client: %s", client.Username)
			client.close()
		}
	}

	for spectator := range r.Spectators {
		select {
		case spectator.Send <- msg:
		default:
			log.Printf("dropping unresponsive spectator: %s", spectator.Username)
			spectator.close()
		}
	}
}

func (r *Room) BroadcastExcept(msg []byte, exclude *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for client := range r.Clients {
		if client != exclude {
			select {
			case client.Send <- msg:
				// message sent successfully
			default:
				log.Printf("dropping unresponsive client: %s", client.Username)
				client.close()
			}
		}
	}

	for spectator := range r.Spectators {
		if spectator != exclude {
			select {
			case spectator.Send <- msg:
			default:
				log.Printf("dropping unresponsive spectator: %s", spectator.Username)
				spectator.close()
			}
		}
	}
}

func (r *Room) Run() {
	for {
		select {
		case client := <-r.Register:
			r.mu.Lock()

			isSpectator := len(r.Clients) >= 4 || client.Spectator

			if isSpectator {
				r.Spectators[client] = true
			} else {
				r.Clients[client] = true

				deck := &Deck{
					ID: client.Username,
					X:  100,
					Y:  100,
				}
				client.Room.Decks[client.Username] = deck

				if r.PlayerPositions == nil {
					r.PlayerPositions = make(map[string]string)
				}
				taken := make(map[string]bool)
				for _, pos := range r.PlayerPositions {
					taken[pos] = true
				}

				var assigned string
				for _, pos := range defaultPositions {
					if !taken[pos] {
						assigned = pos
						break
					}
				}
				if assigned == "" {
					assigned = "unassigned"
				}
				r.PlayerPositions[client.Username] = assigned
				r.HandSizes[client.Username] = 0
			}

			// Prepare cards (common for all clients)
			cards := make([]*BoardCard, 0, len(r.Cards))
			for _, card := range r.Cards {
				cards = append(cards, card)
			}

			// Send board state
			payload := map[string]interface{}{
				"type":        "BOARD_STATE",
				"cards":       cards,
				"decks":       r.Decks,
				"users":       r.GetUsernames(),
				"positions":   r.PlayerPositions,
				"handSizes":   r.HandSizes,
				"turn":        r.Turn,
				"counters":    r.Counters,
				"diceRollers": r.DiceRollers,
			}
			data, _ := json.Marshal(payload)
			client.Send <- data

			r.mu.Unlock()

		case msg := <-r.Broadcast:
			log.Printf("Broadcasting to %d clients", len(r.Clients))
			log.Printf("Broadcasting to %d spectators", len(r.Spectators))
			r.BroadcastSafe(msg)

		case client := <-r.Unregister:
			r.mu.Lock()
			log.Printf("Client %s disconnected", client.Username)
			if _, ok := r.Spectators[client]; ok {
				delete(r.Spectators, client);
				r.mu.Unlock()
			} else if _, ok := r.Clients[client]; ok {
				delete(r.Clients, client)
				delete(r.Decks, client.Username)
				delete(r.DeckURLs, client.Username)
				delete(r.PlayerPositions, client.Username)
				delete(r.HandSizes, client.Username)
				for id, card := range r.Cards {
					if card.Owner == client.Username {
						delete(r.Cards, id)
					}
				}
				if r.Turn == client.Username {
					r.Turn = getNextTurn(r.PlayerPositions, r.Turn)
				}
				payload := map[string]interface{}{
					"type":      "USER_LEFT",
					"user":      client.Username,
					"positions": r.PlayerPositions,
					"turn":      r.Turn,
				}
				data, _ := json.Marshal(payload)
				r.mu.Unlock()
				r.BroadcastSafe(data)
			} else {
				r.mu.Unlock()
			}

			r.mu.Lock()
			if len(r.Clients) == 0 {
				r.mu.Unlock()
				return
			}
			r.mu.Unlock()

		}
	}
}

func (r *Room) GetUsernames() []string {
	usernames := []string{}
	for client := range r.Clients {
		usernames = append(usernames, client.Username)
	}
	return usernames
}

func (r * Room) GetSpectators() []string {
	usernames := []string{}
	for client := range r.Spectators {
		usernames = append(usernames, client.Username)
	}
	return usernames
}

func getNextTurn(positions map[string]string, activePlayer string) string {

	posToPlayer := make(map[string]string)
	for user, pos := range positions {
		posToPlayer[pos] = user
	}
	currentPos := positions[activePlayer]
	currentIdx := -1
	for i, pos := range defaultPositions {
		if pos == currentPos {
			currentIdx = i
			break
		}
	}
	for i := 1; i <= 4; i++ {
		nextIdx := (currentIdx + i) % 4
		nextPos := defaultPositions[nextIdx]
		nextPlayer, exists := posToPlayer[nextPos]
		if exists {
			return nextPlayer
		}
	}

	return ""
}
