package ws

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"log"
	"time"
	"sync"
)

type Client struct {
	Conn     *websocket.Conn
	Send     chan []byte
	Room     *Room
	Username string
    closeOnce sync.Once
}

func (c *Client) close() {
    c.closeOnce.Do(func() {
        c.Room.Unregister <- c
        c.Conn.Close()
        close(c.Send)
    })
}

func (c *Client) read() {
	defer func() {
		c.close()
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
			rawDeckJSON, err := FetchDeckJSON(msg.DeckURL)
			if err != nil {
				log.Printf("error fetching deck: %v", err)
				return
			}
			parsedCards, err := ParseDeck(rawDeckJSON)
			if err != nil {
				log.Printf("error parsing deck: %v", err)
				return
			}
			c.Room.mu.Lock()
			c.Room.DeckURLs[c.Username] = msg.DeckURL
			deck := &Deck{
				ID:    c.Username,
				X:     100,
				Y:     100,
				Cards: parsedCards,
			}
			c.Room.Decks[c.Username] = deck
			c.Room.mu.Unlock()
			payload := map[string]interface{}{
				"type":  "USER_JOINED",
				"users": c.Room.GetUsernames(),
				"decks": c.Room.Decks,
			}
			joinedData, _ := json.Marshal(payload)
			c.Room.BroadcastSafe(joinedData)

		case "MOVE_CARD":
			c.Room.mu.Lock()
			card, exists := c.Room.Cards[msg.ID]
			if !exists {
				card = &BoardCard{ID: msg.ID}
				c.Room.Cards[msg.ID] = card
			}
			card.X = msg.X
			card.Y = msg.Y
			c.Room.mu.Unlock()
			wrapped := map[string]interface{}{
				"type": "MOVE_CARD",
				"id":   card.ID,
				"x":    card.X,
				"y":    card.Y,
			}
			updated, _ := json.Marshal(wrapped)
			c.Room.Broadcast <- updated

		case "MOVE_DECK":
			c.Room.mu.Lock()
			deck, exists := c.Room.Decks[msg.ID]
			if !exists {
				c.Room.mu.Unlock()
				return
			}
			deck.X = msg.X
			deck.Y = msg.Y
			c.Room.mu.Unlock()

			payload := map[string]interface{}{
				"type": "MOVE_DECK",
				"id":   deck.ID,
				"x":    deck.X,
				"y":    deck.Y,
			}
			data, _ := json.Marshal(payload)
			c.Room.Broadcast <- data

		}
	}
}

func (c *Client) write() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.close()
	}()

	for {
		select {
		case msg, ok := <-c.Send:
			if !ok {
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
