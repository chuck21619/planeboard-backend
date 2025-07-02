package ws

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	apiURL, err := WebpageURLToAPIURL(deckURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch deck: %w", err)
	}
	resp, err := http.Get(apiURL)
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
				CollectorNumber string `json:"collectorNumber"`
				Edition         struct {
					EditionCode string `json:"editioncode"`
				} `json:"edition"`
				OracleCard struct {
					Name string `json:"name"`
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
		imageURL := fmt.Sprintf(
			"https://api.scryfall.com/cards/%s/%s?format=image&version=normal",
			c.Card.Edition.EditionCode,
			c.Card.CollectorNumber,
		)

		isCommander := false
		for _, category := range c.Categories {
			if category == "Commander" {
				isCommander = true
				break
			}
		}

		for i := 0; i < c.Quantity; i++ {
			suffix := make([]byte, 4)
			_, err := rand.Read(suffix)
			if err != nil {
				return nil, nil, fmt.Errorf("error generating card ID: %w", err)
			}
			uniqueID := fmt.Sprintf("%d-%s", c.Card.ID, hex.EncodeToString(suffix))
			card := Card{
				ID:       uniqueID,
				Name:     c.Card.OracleCard.Name,
				ImageURL: imageURL,
			}
			if isCommander {
				commanderCards = append(commanderCards, card)
			} else {
				allCards = append(allCards, card)
			}
		}
	}

	return allCards, commanderCards, nil
}
