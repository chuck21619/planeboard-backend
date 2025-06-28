package ws

import (
	"encoding/json"
	"log"
	"sync"
)

var defaultPositions = []string{"topLeft", "topRight", "bottomLeft", "bottomRight"}

type Room struct {
	ID              string
	Clients         map[*Client]bool
	Register        chan *Client
	Unregister      chan *Client
	Broadcast       chan []byte
	Cards           map[string]*BoardCard
	mu              sync.Mutex
	DeckURLs        map[string]string
	Decks           map[string]*Deck
	PlayerPositions map[string]string
	HandSizes       map[string]int
}

func NewRoom(id string) *Room {
	return &Room{
		ID:         id,
		Clients:    make(map[*Client]bool),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		Broadcast:  make(chan []byte),
		Cards:      make(map[string]*BoardCard),
		DeckURLs:   make(map[string]string),
		Decks:      make(map[string]*Deck),
		HandSizes:  make(map[string]int),
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
}

func (r *Room) Run() {
	for {
		select {
		case client := <-r.Register:
			r.mu.Lock()
			r.Clients[client] = true
			cards := make([]*BoardCard, 0, len(r.Cards))
			for _, card := range r.Cards {
				cards = append(cards, card)
			}
			deck := &Deck{
				ID: client.Username,
				X:  100,
				Y:  100,
			}
			client.Room.Decks[client.Username] = deck
			decks := make([]*Deck, 0, len(r.Decks))
			for _, deck := range r.Decks {
				decks = append(decks, deck)
			}
			if r.PlayerPositions == nil {
				r.PlayerPositions = make(map[string]string)
			}
			r.PlayerPositions[client.Username] = defaultPositions[len(r.PlayerPositions)]
			r.HandSizes[client.Username] = 0
			payload := map[string]interface{}{
				"type":      "BOARD_STATE",
				"cards":     cards,
				"decks":     decks,
				"users":     r.GetUsernames(),
				"positions": r.PlayerPositions,
				"handSizes": r.HandSizes,
			}
			data, _ := json.Marshal(payload)
			client.Send <- data
			r.mu.Unlock()

		case msg := <-r.Broadcast:
			log.Printf("Broadcasting to %d clients", len(r.Clients))
			r.BroadcastSafe(msg)

		case client := <-r.Unregister:
			r.mu.Lock()
			log.Printf("Client %s disconnected", client.Username)
			if _, ok := r.Clients[client]; ok {
				delete(r.Clients, client)
				delete(r.Decks, client.Username)
				delete(r.DeckURLs, client.Username)
				delete(r.PlayerPositions, client.Username)
				delete(r.HandSizes, client.Username)
				payload := map[string]interface{}{
					"type":      "USER_LEFT",
					"user":      client.Username,
					"positions": r.PlayerPositions,
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
