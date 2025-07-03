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
	"time"
)

// Import your logging client getter here
// Make sure to add the logging client code in the same package (ws) or import it if external

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
				OracleCard struct {
					Name   string   `json:"name"`
					Tokens []string `json:"tokens"`
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

		var tokens []Card
		if len(c.Card.OracleCard.Tokens) > 0 {
			t, err := FetchTokensForCard(c.Card.UID)
			if err != nil {
				fmt.Printf("Failed to fetch tokens for card %s: %v\n", c.Card.OracleCard.Name, err)
			} else {
				tokens = t
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
				ID:       uniqueID,
				Name:     c.Card.OracleCard.Name,
				ImageURL: imageURL,
				Tokens:   tokens,
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

func FetchTokensForCard(cardUID string) ([]Card, error) {
	client := GetLoggingClient()
	scryfallURL := fmt.Sprintf("https://api.scryfall.com/cards/%s", cardUID)
	resp, err := client.Get(scryfallURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var cardData struct {
		AllParts []struct {
			ID        string `json:"id"`
			Component string `json:"component"`
			Name      string `json:"name"`
			URI       string `json:"uri"`
		} `json:"all_parts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cardData); err != nil {
		return nil, err
	}

	var tokens []Card
	for _, part := range cardData.AllParts {
		if part.Component != "token" {
			continue
		}

		time.Sleep(100 * time.Millisecond)
		tokenResp, err := client.Get(part.URI)
		if err != nil {
			continue
		}
		defer tokenResp.Body.Close()

		var tokenData struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			ImageURIs struct {
				Normal string `json:"normal"`
			} `json:"image_uris"`
		}
		if err := json.NewDecoder(tokenResp.Body).Decode(&tokenData); err != nil {
			continue
		}

		tokens = append(tokens, Card{
			ID:       tokenData.ID,
			Name:     tokenData.Name,
			ImageURL: tokenData.ImageURIs.Normal,
		})
	}

	return tokens, nil
}
