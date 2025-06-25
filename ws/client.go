package ws

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"time"
)

type Client struct {
	Conn     *websocket.Conn
	Send     chan []byte
	Room     *Room
	Username string
}

func (c *Client) read() {
	defer func() {
		c.Room.Unregister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

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
		case "JOIN":
			c.Username = msg.Username
			c.Room.mu.Lock()
			c.Room.DeckURLs[c.Username] = msg.DeckURL
			payload := map[string]interface{}{
				"type":  "USER_JOINED",
				"users": c.Room.GetUsernames(),
				"decks": c.Room.DeckURLs,
			}
			joinedData, _ := json.Marshal(payload)
			c.Room.Broadcast <- joinedData
			c.Room.mu.Unlock()
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
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Room.Unregister <- c
		c.Conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.Send:
			if !ok {
				// Room closed the channel.
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.Conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
