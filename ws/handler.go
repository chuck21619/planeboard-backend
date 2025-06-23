package ws

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Client struct {
	Conn *websocket.Conn
	Send chan []byte
	Room *Room
}

func ServeWebSocket(hub *Hub, w http.ResponseWriter, r *http.Request) {
	roomID := r.URL.Query().Get("room")
	if roomID == "" {
		http.Error(w, "Missing room ID", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	room := hub.GetOrCreateRoom(roomID)
	client := &Client{
		Conn: conn,
		Send: make(chan []byte),
		Room: room,
	}
	room.Register <- client

	go client.read()
	go client.write()
}

type Message struct {
	Type string  `json:"type"`
	ID   string  `json:"id,omitempty"`
	X    float64 `json:"x,omitempty"`
	Y    float64 `json:"y,omitempty"`
}

func (c *Client) read() {
	defer func() {
		c.Room.Unregister <- c
		c.Conn.Close()
	}()
	for {
		_, rawMsg, err := c.Conn.ReadMessage()
		if err != nil {
			break
		}

		var msg Message
		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "MOVE_CARD":
			c.Room.mu.Lock()
			card, exists := c.Room.Cards[msg.ID]
			if !exists {
				card = &Card{ID: msg.ID}
				c.Room.Cards[msg.ID] = card
			}
			card.X = msg.X
			card.Y = msg.Y
			c.Room.mu.Unlock()

			// Broadcast updated card to all clients
			wrapped := map[string]interface{}{
				"type": "MOVE_CARD",
				"id":   card.ID,
				"x":    card.X,
				"y":    card.Y,
			}
			updated, _ := json.Marshal(wrapped)
			c.Room.Broadcast <- updated
		}
	}
}

func (c *Client) write() {
	defer func() {
		c.Room.Unregister <- c
		c.Conn.Close()
	}()
	for msg := range c.Send {
		if err := c.Conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			break
		}
	}
}
