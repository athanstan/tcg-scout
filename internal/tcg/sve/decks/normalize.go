package decks

import (
	"encoding/json"
	"fmt"
)

type Deck struct {
	Title string     `json:"title,omitempty"`
	Craft string     `json:"craft,omitempty"`
	Cards []DeckCard `json:"cards"`
}

type DeckCard struct {
	CardNumber string `json:"card_number"`
	Name       string `json:"name"`
	Total      int    `json:"total"`
	Cost       string `json:"cost,omitempty"`
	Rarity     string `json:"rarity,omitempty"`
	CardType   string `json:"card_type,omitempty"`
	Craft      string `json:"craft,omitempty"`
	Img        string `json:"img,omitempty"`
}

type decklogPayload struct {
	Title       string           `json:"title"`
	DeckParam2  string           `json:"deck_param2"`
	List        []decklogCardRaw `json:"list"`
	LeaderList  []decklogCardRaw `json:"p_list"`
	EvolvedList []decklogCardRaw `json:"sub_list"`
}

type decklogCardRaw struct {
	CardNumber  string `json:"card_number"`
	Name        string `json:"name"`
	Num         int    `json:"num"`
	Cost        string `json:"cost"`
	Rare        string `json:"rare"`
	CardKind    string `json:"card_kind"`
	Affiliation string `json:"affiliation"`
	Img         string `json:"img"`
}

func NormalizeDeck(raw json.RawMessage) (Deck, error) {
	var payload decklogPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return Deck{}, fmt.Errorf("decode decklog payload: %w", err)
	}

	cards := make([]DeckCard, 0, len(payload.LeaderList)+len(payload.List)+len(payload.EvolvedList))
	for _, card := range payload.LeaderList {
		cards = append(cards, normalizeDeckCard(card, "Leader"))
	}
	for _, card := range payload.List {
		cards = append(cards, normalizeDeckCard(card, ""))
	}
	for _, card := range payload.EvolvedList {
		cards = append(cards, normalizeDeckCard(card, ""))
	}

	return Deck{
		Title: payload.Title,
		Craft: payload.DeckParam2,
		Cards: cards,
	}, nil
}

func normalizeDeckCard(card decklogCardRaw, rarityOverride string) DeckCard {
	rarity := card.Rare
	if rarityOverride != "" {
		rarity = rarityOverride
	}

	return DeckCard{
		CardNumber: card.CardNumber,
		Name:       card.Name,
		Total:      card.Num,
		Cost:       card.Cost,
		Rarity:     rarity,
		CardType:   card.CardKind,
		Craft:      card.Affiliation,
		Img:        card.Img,
	}
}
