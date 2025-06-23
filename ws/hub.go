package ws

import (
	"sync"
	"encoding/json"
    "log"
)

type Hub struct {
	Rooms map[string]*Room
	mu    sync.Mutex
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

type Room struct {
	ID        string
	Clients   map[*Client]bool
	Register  chan *Client
	Broadcast chan []byte
	Cards     map[string]*Card
	mu        sync.Mutex
}

func NewRoom(id string) *Room {
	return &Room{
		ID:        id,
		Clients:   make(map[*Client]bool),
		Register:  make(chan *Client),
		Broadcast: make(chan []byte),
		Cards:     make(map[string]*Card),
	}
}

func (r *Room) Run() {
	for {
		select {
		case client := <-r.Register:
			r.mu.Lock()
			r.Clients[client] = true

			// send current cards to new client
			for _, card := range r.Cards {
				data, _ := json.Marshal(card)
				client.Send <- data
			}
			r.mu.Unlock()

		case msg := <-r.Broadcast:
			r.mu.Lock()
    		log.Printf("Broadcasting to %d clients", len(r.Clients))
			for c := range r.Clients {
				c.Send <- msg
			}
			r.mu.Unlock()
		}
	}
}


type Card struct {
	ID string  `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
}
