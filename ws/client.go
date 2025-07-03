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
				c.sendError("Error fetching deck")
				return
			}
			parsedCards, parsedCommanders, err := ParseDeck(rawDeckJSON)
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
				ID:         c.Username,
				X:          x,
				Y:          y,
				Cards:      parsedCards,
				Commanders: parsedCommanders,
			}
			c.Room.Decks[c.Username] = deck
			commanderYOffset := 100.0
			if pos == "bottomLeft" || pos == "bottomRight" {
				commanderYOffset = -100.0
			}
			commanderXOffsetSign := -1.0
			switch pos {
			case "topRight", "bottomRight":
				commanderXOffsetSign = 1.0
			}
			var commanderBoardCards []*BoardCard
			for i, commander := range parsedCommanders {
				card := &BoardCard{
					ID:       commander.ID,
					Name:     commander.Name,
					ImageURL: commander.ImageURL,
					X:        x + commanderXOffsetSign*float64(i)*70,
					Y:        y + commanderYOffset,
					Owner:    c.Username,
					Tapped:   false,
				}
				c.Room.Cards[commander.ID] = card
				commanderBoardCards = append(commanderBoardCards, card)
			}

			c.Room.mu.Unlock()
			payload := map[string]interface{}{
				"type":       "USER_JOINED",
				"users":      c.Room.GetUsernames(),
				"decks":      c.Room.Decks,
				"positions":  c.Room.PlayerPositions,
				"commanders": commanderBoardCards,
			}
			joinedData, _ := json.Marshal(payload)
			c.Room.BroadcastSafe(joinedData)
		case "DRAW_CARD":
			deck, ok := c.Room.Decks[c.Username]
			if !ok || len(deck.Cards) == 0 {
				return
			}
			deck.Cards = deck.Cards[1:]
			c.Room.HandSizes[c.Username] += 1
			update := map[string]interface{}{
				"type":     "PLAYER_DREW_CARD",
				"player":   c.Username,
				"handSize": c.Room.HandSizes[c.Username],
			}
			broadcast, _ := json.Marshal(update)
			c.Room.BroadcastExcept(broadcast, c)
		case "CARD_PLAYED_FROM_HAND":
			c.Room.mu.Lock()
			card := &BoardCard{
				ID:       msg.Card.ID,
				Name:     msg.Card.Name,
				ImageURL: msg.Card.ImageURL,
				X:        msg.Card.X,
				Y:        msg.Card.Y,
				Owner:    c.Username,
			}
			c.Room.Cards[card.ID] = card
			c.Room.HandSizes[c.Username] -= 1
			handSize := c.Room.HandSizes[c.Username]
			c.Room.mu.Unlock()
			broadcast := map[string]interface{}{
				"type":     "CARD_PLAYED_FROM_HAND",
				"card":     card,
				"player":   c.Username,
				"handSize": handSize,
			}
			data, _ := json.Marshal(broadcast)
			c.Room.BroadcastExcept(data, c)
		case "CARD_PLAYED_FROM_LIBRARY":
			c.Room.mu.Lock()
			card := &BoardCard{
				ID:       msg.Card.ID,
				Name:     msg.Card.Name,
				ImageURL: msg.Card.ImageURL,
				X:        msg.Card.X,
				Y:        msg.Card.Y,
				Owner:    msg.Username,
			}
			c.Room.Cards[card.ID] = card
			if deck, ok := c.Room.Decks[msg.Username]; ok {
				filteredCards := deck.Cards[:0]
				for _, dcard := range deck.Cards {
					if dcard.ID != card.ID {
						filteredCards = append(filteredCards, dcard)
					}
				}
				deck.Cards = filteredCards
			}
			c.Room.mu.Unlock()
			broadcast := map[string]interface{}{
				"type":   "CARD_PLAYED_FROM_LIBRARY",
				"card":   card,
				"player": msg.Username,
			}
			data, _ := json.Marshal(broadcast)
			c.Room.BroadcastExcept(data, c)
		case "TAP_CARD":
			c.Room.mu.Lock()
			card, exists := c.Room.Cards[msg.ID]
			if !exists {
				card = &BoardCard{ID: msg.ID}
				c.Room.Cards[msg.ID] = card
			}
			card.Tapped = msg.Tapped
			c.Room.mu.Unlock()
			wrapped := map[string]interface{}{
				"type":   "CARD_TAPPED",
				"id":     card.ID,
				"tapped": card.Tapped,
			}
			updated, _ := json.Marshal(wrapped)
			c.Room.BroadcastExcept(updated, c)

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
			c.Room.BroadcastExcept(updated, c)
		case "TUTOR_TO_HAND":
			c.Room.mu.Lock()
			c.Room.HandSizes[msg.Username] += 1
			handSize := c.Room.HandSizes[msg.Username]
			if deck, ok := c.Room.Decks[msg.Username]; ok {
				filteredCards := deck.Cards[:0]
				for _, dcard := range deck.Cards {
					if dcard.ID != msg.ID {
						filteredCards = append(filteredCards, dcard)
					}
				}
				deck.Cards = filteredCards
			}
			c.Room.mu.Unlock()
			broadcast := map[string]interface{}{
				"type":     "TUTORED_TO_HAND",
				"id":       msg.ID,
				"player":   msg.Username,
				"handSize": handSize,
			}
			data, err := json.Marshal(broadcast)
			if err == nil {
				c.Room.BroadcastExcept(data, c)
			}
		case "RETURN_TO_HAND":
			c.Room.mu.Lock()
			delete(c.Room.Cards, msg.ID)
			c.Room.HandSizes[msg.Username] += 1
			handSize := c.Room.HandSizes[msg.Username]
			c.Room.mu.Unlock()

			broadcast := map[string]interface{}{
				"type":     "RETURN_TO_HAND",
				"id":       msg.ID,
				"player":   msg.Username,
				"handSize": handSize,
			}
			data, err := json.Marshal(broadcast)
			if err == nil {
				c.Room.BroadcastExcept(data, c)
			}
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
