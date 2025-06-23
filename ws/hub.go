package ws

import (
	"encoding/json"
	"log"
	"sync"
)

type Hub struct {
	Rooms map[string]*Room
	mu    sync.Mutex
}

type Card struct {
	ID string  `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
}

type Room struct {
	ID         string
	Clients    map[*Client]bool
	Register   chan *Client
	Unregister chan *Client
	Broadcast  chan []byte
	Cards      map[string]*Card
	mu         sync.Mutex
}

func NewHub() *Hub {
	return &Hub{
		Rooms: make(map[string]*Room),
	}
}

func (h *Hub) GetOrCreateRoom(id string) *Room {
	h.mu.Lock()
	defer h.mu.Unlock()

	room, exists := h.Rooms[id]
	if !exists {
		room = NewRoom(id)
		h.Rooms[id] = room
		go room.Run()
	}
	return room
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
	}
}

func (r *Room) Run() {
	for {
		select {
		case client := <-r.Register:
			r.mu.Lock()
			r.Clients[client] = true

			// send current cards to new client
			cards := make([]*Card, 0, len(r.Cards))
			for _, card := range r.Cards {
				cards = append(cards, card)
			}
			payload := map[string]interface{}{
				"type":  "BOARD_STATE",
				"cards": cards,
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
			if _, ok := r.Clients[client]; ok {
				delete(r.Clients, client)
				close(client.Send) // 💥 closes writer goroutine
			}
			r.mu.Unlock()
		}
	}
}
