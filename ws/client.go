package ws

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"log"
	"sync"
	"time"
)

type Client struct {
	Conn      *websocket.Conn
	Send      chan []byte
	Room      *Room
	Username  string
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
				c.sendError("Invalid deck URL")
				return
			}
			parsedCards, err := ParseDeck(rawDeckJSON)
			if err != nil {
				log.Printf("error parsing deck: %v", err)
				return
			}
			c.Room.mu.Lock()
			c.Room.DeckURLs[c.Username] = msg.DeckURL
			pos := c.Room.PlayerPositions[c.Username]
			var x, y float64
			deckWidth := 60.0
			deckHeight := 90.0
			xOffset := 50.0
			yOffset := 175.0
			switch pos {
			case "topLeft":
				x = -xOffset - deckWidth/2
				y = -yOffset - deckHeight/2
			case "topRight":
				x = xOffset - deckWidth/2
				y = -yOffset - deckHeight/2
			case "bottomLeft":
				x = -xOffset - deckWidth/2
				y = yOffset - deckHeight/2
			case "bottomRight":
				x = xOffset - deckWidth/2
				y = yOffset - deckHeight/2
			default:
				x, y = 0, 0
			}
			deck := &Deck{
				ID:    c.Username,
				X:     x,
				Y:     y,
				Cards: parsedCards,
			}
			c.Room.Decks[c.Username] = deck
			c.Room.mu.Unlock()
			payload := map[string]interface{}{
				"type":      "USER_JOINED",
				"users":     c.Room.GetUsernames(),
				"decks":     c.Room.Decks,
				"positions": c.Room.PlayerPositions,
			}
			joinedData, _ := json.Marshal(payload)
			c.Room.BroadcastSafe(joinedData)
		case "DRAW_CARD":
			deck, ok := c.Room.Decks[c.Username]
			if !ok || len(deck.Cards) == 0 {
				return
			}
			card := deck.Cards[0]
			deck.Cards = deck.Cards[1:]
			msg := map[string]interface{}{
				"type": "CARD_DRAWN",
				"card": card,
			}
			data, _ := json.Marshal(msg)
			c.Send <- data
			update := map[string]interface{}{
				"type":   "PLAYER_DREW_CARD",
				"player": c.Username,
			}
			broadcast, _ := json.Marshal(update)
			c.Room.BroadcastSafe(broadcast)

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
				log.Printf("Send channel closed, exiting write goroutine for user %s", c.Username)
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.Conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				log.Printf("Error writing to websocket for user %s: %v", c.Username, err)
				return
			} else {
				log.Printf("Sent message to user %s", c.Username)
			}
		case <-ticker.C:
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("Ping error for user %s: %v", c.Username, err)
				return
			}
		}
	}
}

func (c *Client) sendError(reason string) {
	msg := map[string]string{
		"type":   "ERROR",
		"reason": reason,
	}
	if data, err := json.Marshal(msg); err == nil {
		select {
		case c.Send <- data:
			time.Sleep(100 * time.Millisecond)
		default:
			log.Printf("unable to send error to client: channel blocked")
		}
	}
}
