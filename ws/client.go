package ws

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"log"
	mrand "math/rand"
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
			yOffset := 225.0
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
					Card:      commander,
					X:         x + commanderXOffsetSign*float64(i)*70,
					Y:         y + commanderYOffset,
					Owner:     c.Username,
					Tapped:    false,
					FlipIndex: 0,
				}
				c.Room.Cards[commander.ID] = card
				commanderBoardCards = append(commanderBoardCards, card)
			}
			c.Room.LifeTotals[c.Username] = 40
			c.Room.mu.Unlock()
			payload := map[string]interface{}{
				"type":       "USER_JOINED",
				"users":      c.Room.GetUsernames(),
				"decks":      c.Room.Decks,
				"positions":  c.Room.PlayerPositions,
				"commanders": commanderBoardCards,
				"lifeTotals": c.Room.LifeTotals,
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

		case "PASS_TURN":
			c.Room.mu.Lock()
			c.Room.Turn = getNextTurn(c.Room.PlayerPositions, c.Room.Turn)
			c.Room.mu.Unlock()
			update := map[string]interface{}{
				"type": "TURN_PASSED",
				"turn": c.Room.Turn,
			}
			broadcast, _ := json.Marshal(update)
			c.Room.BroadcastSafe(broadcast)

		case "UNTAP_ALL":
			c.Room.mu.Lock()
			for _, card := range c.Room.Cards {
				if card.Owner == c.Username {
					card.Tapped = false
				}
			}
			c.Room.mu.Unlock()
			update := map[string]interface{}{
				"type":   "UNTAPPED_ALL",
				"player": c.Username,
			}
			broadcast, _ := json.Marshal(update)
			c.Room.BroadcastExcept(broadcast, c)

		case "CARD_TO_TOP_OF_DECK":
			c.Room.mu.Lock()
			deck := c.Room.Decks[msg.Username]
			if msg.Source == "board" {
				delete(c.Room.Cards, msg.Card.ID)
			} else if msg.Source == "hand" {
				c.Room.HandSizes[msg.Username] -= 1
			} else {
				return
			}
			card := Card{
				ID:           msg.Card.ID,
				Name:         msg.Card.Name,
				ImageURL:     msg.Card.ImageURL,
				ImageURLBack: msg.Card.ImageURLBack,
				UID:          msg.Card.UID,
				HasTokens:    msg.Card.HasTokens,
				NumFaces:     msg.Card.NumFaces,
			}
			deck.Cards = append([]Card{card}, deck.Cards...)
			c.Room.mu.Unlock()
			update := map[string]interface{}{
				"username":  c.Username,
				"type":      "CARD_TO_TOP_OF_DECK",
				"deckId":    msg.Username,
				"deckCards": deck.Cards,
				"handSize":  c.Room.HandSizes,
				"id":        msg.Card.ID,
				"source":    msg.Source,
			}
			broadcast, _ := json.Marshal(update)
			c.Room.BroadcastExcept(broadcast, c)

		case "CARD_TO_BOTTOM_OF_DECK":
			c.Room.mu.Lock()
			deck := c.Room.Decks[msg.Username]
			if msg.Source == "board" {
				delete(c.Room.Cards, msg.Card.ID)
			} else if msg.Source == "hand" {
				c.Room.HandSizes[msg.Username] -= 1
			} else {
				return
			}
			card := Card{
				ID:           msg.Card.ID,
				Name:         msg.Card.Name,
				ImageURL:     msg.Card.ImageURL,
				ImageURLBack: msg.Card.ImageURLBack,
				UID:          msg.Card.UID,
				HasTokens:    msg.Card.HasTokens,
				NumFaces:     msg.Card.NumFaces,
			}
			deck.Cards = append(deck.Cards, card)
			c.Room.mu.Unlock()
			update := map[string]interface{}{
				"username":  c.Username,
				"type":      "CARD_TO_BOTTOM_OF_DECK",
				"deckId":    msg.Username,
				"deckCards": deck.Cards,
				"handSize":  c.Room.HandSizes,
				"id":        msg.Card.ID,
				"source":    msg.Source,
			}
			broadcast, _ := json.Marshal(update)
			c.Room.BroadcastExcept(broadcast, c)

		case "CARD_TO_SHUFFLE_IN_DECK":
			c.Room.mu.Lock()
			deck := c.Room.Decks[msg.Username]
			if msg.Source == "board" {
				delete(c.Room.Cards, msg.Card.ID)
			} else if msg.Source == "hand" {
				c.Room.HandSizes[msg.Username] -= 1
			} else {
				return
			}
			card := Card{
				ID:           msg.Card.ID,
				Name:         msg.Card.Name,
				ImageURL:     msg.Card.ImageURL,
				ImageURLBack: msg.Card.ImageURLBack,
				UID:          msg.Card.UID,
				HasTokens:    msg.Card.HasTokens,
				NumFaces:     msg.Card.NumFaces,
			}
			deck.Cards = append(deck.Cards, card)
			r := mrand.New(mrand.NewSource(time.Now().UnixNano()))
			r.Shuffle(len(deck.Cards), func(i, j int) {
				deck.Cards[i], deck.Cards[j] = deck.Cards[j], deck.Cards[i]
			})
			c.Room.mu.Unlock()
			update := map[string]interface{}{
				"username":  c.Username,
				"type":      "CARD_TO_SHUFFLE_IN_DECK",
				"deckId":    msg.Username,
				"deckCards": deck.Cards,
				"handSize":  c.Room.HandSizes,
				"id":        msg.Card.ID,
				"source":    msg.Source,
			}
			broadcast, _ := json.Marshal(update)
			c.Room.BroadcastSafe(broadcast)

		case "CARD_PLAYED_FROM_HAND":
			c.Room.mu.Lock()
			card := &BoardCard{
				Card: Card{
					ID:           msg.Card.ID,
					Name:         msg.Card.Name,
					ImageURL:     msg.Card.ImageURL,
					ImageURLBack: msg.Card.ImageURLBack,
					UID:          msg.Card.UID,
					HasTokens:    msg.Card.HasTokens,
					NumFaces:     msg.Card.NumFaces,
				},
				X:         msg.Card.X,
				Y:         msg.Card.Y,
				Owner:     c.Username,
				Tapped:    false,
				FlipIndex: msg.Card.FlipIndex,
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

		case "LIFE_TOTAL_CHANGE":
			c.Room.mu.Lock()
			c.Room.LifeTotals[msg.Username] = *msg.LifeTotal
			c.Room.mu.Unlock()
			broadcast := map[string]interface{}{
				"type":      "LIFE_TOTAL_UPDATED",
				"username":  msg.Username,
				"lifeTotal": *msg.LifeTotal,
			}
			data, _ := json.Marshal(broadcast)
			c.Room.BroadcastExcept(data, c)

		case "SPAWN_TOKEN":
			c.Room.mu.Lock()
			token := &BoardCard{
				Card: Card{
					ID:        msg.Card.ID,
					Name:      msg.Card.Name,
					ImageURL:  msg.Card.ImageURL,
					UID:       msg.Card.UID,
					HasTokens: msg.Card.HasTokens,
					NumFaces:  msg.Card.NumFaces,
				},
				X:         msg.Card.X,
				Y:         msg.Card.Y,
				Owner:     msg.Card.Owner,
				Tapped:    false,
				FlipIndex: 0,
			}
			c.Room.Cards[token.ID] = token
			c.Room.mu.Unlock()
			broadcast := map[string]interface{}{
				"type":  "SPAWN_TOKEN",
				"token": token,
			}
			data, _ := json.Marshal(broadcast)
			c.Room.BroadcastExcept(data, c)

		case "CARD_PLAYED_FROM_LIBRARY":
			c.Room.mu.Lock()
			card := &BoardCard{
				Card: Card{
					ID:           msg.Card.ID,
					Name:         msg.Card.Name,
					ImageURL:     msg.Card.ImageURL,
					ImageURLBack: msg.Card.ImageURLBack,
					UID:          msg.Card.UID,
					HasTokens:    msg.Card.HasTokens,
					NumFaces:     msg.Card.NumFaces,
				},
				X:         msg.Card.X,
				Y:         msg.Card.Y,
				Owner:     c.Username,
				Tapped:    false,
				FlipIndex: msg.Card.FlipIndex,
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
			card := c.Room.Cards[msg.ID]
			card.Tapped = msg.Tapped
			c.Room.mu.Unlock()
			wrapped := map[string]interface{}{
				"type":   "CARD_TAPPED",
				"id":     card.ID,
				"tapped": card.Tapped,
			}
			updated, _ := json.Marshal(wrapped)
			c.Room.BroadcastExcept(updated, c)

		case "FLIP_CARD":
			c.Room.mu.Lock()
			card := c.Room.Cards[msg.ID]
			card.FlipIndex = msg.FlipIndex
			c.Room.mu.Unlock()

			wrapped := map[string]interface{}{
				"type":      "CARD_FLIPPED",
				"id":        card.ID,
				"flipIndex": card.FlipIndex,
			}
			updated, _ := json.Marshal(wrapped)
			c.Room.BroadcastExcept(updated, c)

		case "MOVE_CARD":
			c.Room.mu.Lock()
			card := c.Room.Cards[msg.ID]
			card.X = msg.X
			card.Y = msg.Y
			c.Room.mu.Unlock()
			wrapped := map[string]interface{}{
				"type":      "MOVE_CARD",
				"id":        card.ID,
				"x":         card.X,
				"y":         card.Y,
				"flipIndex": msg.FlipIndex,
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

		case "SCRY_RESOLVED":
			c.Room.mu.Lock()
			c.Room.Decks[msg.Deck.ID] = &msg.Deck
			c.Room.mu.Unlock()
			broadcast := map[string]interface{}{
				"type": "PLAYER_SCRYED",
				"deck": c.Room.Decks[msg.Deck.ID],
			}
			data, _ := json.Marshal(broadcast)
			c.Room.BroadcastExcept(data, c)

		case "SURVEIL_RESOLVED":
			c.Room.mu.Lock()
			c.Room.Decks[msg.Deck.ID] = &msg.Deck
			for _, card := range msg.Cards {
				c.Room.Cards[card.ID] = &card
			}
			c.Room.mu.Unlock()
			broadcast := map[string]interface{}{
				"type":        "PLAYER_SURVEILED",
				"deck":        c.Room.Decks[msg.Deck.ID],
				"toGraveyard": msg.Cards,
			}
			data, _ := json.Marshal(broadcast)
			c.Room.BroadcastExcept(data, c)

		case "ADD_COUNTER":
			c.Room.mu.Lock()
			counter := &Counter{
				ID:    msg.Counters[0].ID,
				X:     msg.Counters[0].X,
				Y:     msg.Counters[0].Y,
				Count: msg.Counters[0].Count,
				Owner: msg.Counters[0].Owner,
			}
			c.Room.Counters[counter.ID] = counter
			c.Room.mu.Unlock()
			broadcast := map[string]interface{}{
				"type":    "COUNTER_ADDED",
				"counter": counter,
			}
			data, _ := json.Marshal(broadcast)
			c.Room.BroadcastExcept(data, c)

		case "MOVE_COUNTER":
			c.Room.mu.Lock()
			counter := c.Room.Counters[msg.ID]
			counter.X = msg.X
			counter.Y = msg.Y
			c.Room.mu.Unlock()
			wrapped := map[string]interface{}{
				"type": "COUNTER_MOVED",
				"id":   counter.ID,
				"x":    counter.X,
				"y":    counter.Y,
			}
			updated, _ := json.Marshal(wrapped)
			c.Room.BroadcastExcept(updated, c)

		case "UPDATE_COUNTER":
			c.Room.mu.Lock()
			counter := c.Room.Counters[msg.ID]
			counter.Count = msg.Count
			c.Room.mu.Unlock()
			wrapped := map[string]interface{}{
				"type":  "COUNTER_UPDATED",
				"id":    counter.ID,
				"count": counter.Count,
			}
			updated, _ := json.Marshal(wrapped)
			c.Room.BroadcastExcept(updated, c)

		case "DELETE_COUNTER":
			c.Room.mu.Lock()
			delete(c.Room.Counters, msg.ID)
			c.Room.mu.Unlock()
			wrapped := map[string]interface{}{
				"type": "COUNTER_DELETED",
				"id":   msg.ID,
			}
			updated, _ := json.Marshal(wrapped)
			c.Room.BroadcastExcept(updated, c)

		case "ADD_DICE_ROLLER":
			c.Room.mu.Lock()
			diceRoller := &DiceRoller{
				ID:       msg.DiceRoller[0].ID,
				X:        msg.DiceRoller[0].X,
				Y:        msg.DiceRoller[0].Y,
				NumDice:  msg.DiceRoller[0].NumDice,
				NumSides: msg.DiceRoller[0].NumSides,
			}
			c.Room.DiceRollers[diceRoller.ID] = diceRoller
			c.Room.mu.Unlock()
			broadcast := map[string]interface{}{
				"type":       "DICE_ROLLER_ADDED",
				"diceRoller": diceRoller,
			}
			data, _ := json.Marshal(broadcast)
			c.Room.BroadcastExcept(data, c)

		case "MOVE_DICE_ROLLER":
			log.Printf("MOVE_DICE_ROLLER: msg.X = %f, msg.Y = %f\n", msg.X, msg.Y)
			c.Room.mu.Lock()
			diceRoller := c.Room.DiceRollers[msg.ID]
			diceRoller.X = msg.X
			diceRoller.Y = msg.Y
			c.Room.mu.Unlock()
			wrapped := map[string]interface{}{
				"type": "DICE_ROLLER_MOVED",
				"id":   diceRoller.ID,
				"x":    diceRoller.X,
				"y":    diceRoller.Y,
			}
			updated, _ := json.Marshal(wrapped)
			c.Room.BroadcastExcept(updated, c)

		case "DELETE_DICE_ROLLER":
			c.Room.mu.Lock()
			delete(c.Room.DiceRollers, msg.ID)
			c.Room.mu.Unlock()
			wrapped := map[string]interface{}{
				"type": "DICE_ROLLER_DELETED",
				"id":   msg.ID,
			}
			updated, _ := json.Marshal(wrapped)
			c.Room.BroadcastExcept(updated, c)

			//case "ROLL_DICE":
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
