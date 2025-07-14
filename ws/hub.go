package ws

import (
	"log"
	"sync"
)

type Hub struct {
	Rooms map[string]*Room
	Mu    sync.Mutex
}

func NewHub() *Hub {
	return &Hub{
		Rooms: make(map[string]*Room),
	}
}

func (h *Hub) GetOrCreateRoom(id string) *Room {
	h.Mu.Lock()
	defer h.Mu.Unlock()

	room, exists := h.Rooms[id]
	if !exists {
		room = NewRoom(id)
		h.Rooms[id] = room

		go func() {
			room.Run()
			h.Mu.Lock()
			delete(h.Rooms, id)
			h.Mu.Unlock()
			log.Printf("Room %s deleted", id)
		}()
	}
	return room
}
