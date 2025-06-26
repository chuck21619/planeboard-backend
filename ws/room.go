package ws

import (
	"encoding/json"
	"log"
	"sync"
)

type Room struct {
	ID         string
	Clients    map[*Client]bool
	Register   chan *Client
	Unregister chan *Client
	Broadcast  chan []byte
	Cards      map[string]*Card
	mu         sync.Mutex
	DeckURLs   map[string]string
	Decks      map[string]*Deck
}

func NewRoom(id string) *Room {
	return &Room{
		ID:         id,
		Clients:    make(map[*Client]bool),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		Broadcast:  make(chan []byte),
		Cards: map[string]*Card{
			"card1": {ID: "card1", X: 100, Y: 100},
		},
		DeckURLs: make(map[string]string),
		Decks:    make(map[string]*Deck),
	}
}

func (r *Room) Run() {
	for {
		select {
		case client := <-r.Register:
			r.mu.Lock()
			r.Clients[client] = true
			cards := make([]*Card, 0, len(r.Cards))
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
			payload := map[string]interface{}{
				"type":  "BOARD_STATE",
				"cards": cards,
				"decks": decks,
				"users": r.GetUsernames(),
			}
			data, _ := json.Marshal(payload)
			client.Send <- data
			r.mu.Unlock()

		case msg := <-r.Broadcast:
			r.mu.Lock()
			log.Printf("Broadcasting to %d clients", len(r.Clients))
			for c := range r.Clients {
				c.Send <- msg
			}
			r.mu.Unlock()

		case client := <-r.Unregister:
			r.mu.Lock()
			log.Printf("Client %s disconnected", client.Username)
			if _, ok := r.Clients[client]; ok {
				delete(r.Clients, client)
				delete(r.Decks, client.Username)
				delete(r.DeckURLs, client.Username)
				close(client.Send)
				payload := map[string]interface{}{
					"type": "USER_LEFT",
					"user": client.Username,
				}
				data, _ := json.Marshal(payload)
				for c := range r.Clients {
					c.Send <- data
				}
			}
			if len(r.Clients) == 0 {
				r.mu.Unlock()
				return
			} else {
				r.mu.Unlock()
			}
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
