package ws

import (
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

		go func() {
			room.Run()
			h.mu.Lock()
			delete(h.Rooms, id)
			h.mu.Unlock()
			log.Printf("Room %s deleted", id)
		}()
	}
	return room
}
