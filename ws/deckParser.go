package ws

import (
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	mrand "math/rand"
	"net/http"
	"strings"
)

func WebpageURLToAPIURL(webpageURL string) (string, error) {
	parts := strings.Split(webpageURL, "/")
	if len(parts) < 5 {
		return "", errors.New("invalid deck webpage URL")
	}
	if parts[3] != "decks" {
		return "", errors.New("URL is not a deck URL")
	}
	deckID := parts[4]
	apiURL := fmt.Sprintf("https://archidekt.com/api/decks/%s/", deckID)
	return apiURL, nil
}

func FetchDeckJSON(deckURL string) ([]byte, error) {
	client := GetLoggingClient()
	apiURL, err := WebpageURLToAPIURL(deckURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch deck: %w", err)
	}
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch deck: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("non-200 response: %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func ParseDeck(data []byte) ([]Card, []Card, error) {
	var parsed struct {
		Cards []struct {
			ID         int64    `json:"id"`
			Quantity   int      `json:"quantity"`
			Categories []string `json:"categories"`
			Card       struct {
				ID              int64  `json:"id"`
				UID             string `json:"uid"`
				CollectorNumber string `json:"collectorNumber"`
				Edition         struct {
					EditionCode string `json:"editioncode"`
				} `json:"edition"`
				ScryfallImageHash string `json:"scryfallImageHash"`
				OracleCard        struct {
					Name   string   `json:"name"`
					Tokens []string `json:"tokens"`
					Layout string   `json:"layout"`
				} `json:"oracleCard"`
			} `json:"card"`
		} `json:"cards"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, nil, fmt.Errorf("error unmarshaling deck JSON: %w", err)
	}
	var allCards []Card
	var commanderCards []Card
	for _, c := range parsed.Cards {
		skip := false
		for _, category := range c.Categories {
			if category == "Maybeboard" || category == "Sideboard" {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		imageURL := fmt.Sprintf(
			"https://api.scryfall.com/cards/%s/%s?format=image&version=normal",
			c.Card.Edition.EditionCode,
			c.Card.CollectorNumber,
		)
		nonStandardBackLayouts := map[string]bool{
			"modal_dfc": true,
			"transform": true,
			"meld":      true,
		}
		imageURLBack := ""
		if nonStandardBackLayouts[c.Card.OracleCard.Layout] && c.Card.UID != "" && c.Card.ScryfallImageHash != "" {
			uidClean := strings.ReplaceAll(c.Card.UID, "-", "")
			if len(uidClean) >= 2 {
				dir1 := string(uidClean[0])
				dir2 := string(uidClean[1])
				imageURLBack = fmt.Sprintf(
					"https://cards.scryfall.io/normal/back/%s/%s/%s.jpg?%s",
					dir1, dir2, c.Card.UID, c.Card.ScryfallImageHash,
				)
			}
		}
		isCommander := false
		for _, category := range c.Categories {
			if category == "Commander" {
				isCommander = true
				break
			}
		}
		for i := 0; i < c.Quantity; i++ {
			suffix := make([]byte, 4)
			_, err := crand.Read(suffix)
			if err != nil {
				return nil, nil, fmt.Errorf("error generating card ID: %w", err)
			}
			uniqueID := fmt.Sprintf("%d-%s", c.Card.ID, hex.EncodeToString(suffix))
			card := Card{
				ID:           uniqueID,
				Name:         c.Card.OracleCard.Name,
				ImageURL:     imageURL,
				ImageURLBack: imageURLBack,
				UID:          c.Card.UID,
				HasTokens:    len(c.Card.OracleCard.Tokens) > 0,
			}
			if isCommander {
				commanderCards = append(commanderCards, card)
			} else {
				allCards = append(allCards, card)
			}
		}
	}
	mrand.Shuffle(len(allCards), func(i, j int) {
		allCards[i], allCards[j] = allCards[j], allCards[i]
	})
	return allCards, commanderCards, nil
}
