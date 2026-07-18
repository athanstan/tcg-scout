package decks

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeDeck(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{
		"can_edit": 0,
		"deck_id": "5RC61",
		"deck_param2": "Dragoncraft",
		"id": 1142863,
		"title": "Test Deck",
		"p_list": [
			{
				"card_number": "BP01-LD07EN",
				"name": "Rowen",
				"num": 1,
				"cost": "-",
				"rare": "-",
				"card_kind": "Leader",
				"affiliation": "Dragoncraft",
				"img": "BP01/BP01-LD07EN.png"
			}
		],
		"sub_list": [
			{
				"card_number": "BP17-001EN",
				"name": "Arisa, Evergreen Arrow",
				"num": 2,
				"cost": "-",
				"rare": "Legendary",
				"card_kind": "Follower Evolved",
				"affiliation": "Forestcraft",
				"img": "BP17/BP17-001EN.png"
			}
		],
		"list": [
			{
				"card_number": "BP17-001EN",
				"name": "Arisa",
				"num": 3,
				"_num": 3,
				"cost": "1",
				"rare": "Legendary",
				"card_kind": "Follower",
				"affiliation": "Forestcraft",
				"img": "BP17/BP17-001EN.png",
				"g_param": {"g0": "1", "g1": 1},
				"p_param": {"p1": "0"},
				"height": 641,
				"width": 459,
				"id": 5303,
				"slot": 0,
				"type": 1
			}
		]
	}`)

	deck, err := NormalizeDeck(raw)
	if err != nil {
		t.Fatalf("NormalizeDeck() error = %v", err)
	}

	if deck.Title != "Test Deck" || deck.Craft != "Dragoncraft" {
		t.Fatalf("deck metadata = %#v", deck)
	}
	if len(deck.Cards) != 3 {
		t.Fatalf("len(cards) = %d, want 3", len(deck.Cards))
	}

	leader := deck.Cards[0]
	if leader.CardNumber != "BP01-LD07EN" || leader.Name != "Rowen" || leader.Rarity != "Leader" {
		t.Fatalf("leader card = %#v", leader)
	}

	card := deck.Cards[1]
	if card.CardNumber != "BP17-001EN" || card.Total != 3 || card.Name != "Arisa" {
		t.Fatalf("card = %#v", card)
	}
	if card.Rarity != "Legendary" || card.CardType != "Follower" || card.Craft != "Forestcraft" {
		t.Fatalf("card metadata = %#v", card)
	}

	evolved := deck.Cards[2]
	if evolved.CardNumber != "BP17-001EN" || evolved.Total != 2 || evolved.Name != "Arisa, Evergreen Arrow" {
		t.Fatalf("evolved card = %#v", evolved)
	}
	if evolved.CardType != "Follower Evolved" || evolved.Rarity != "Legendary" {
		t.Fatalf("evolved metadata = %#v", evolved)
	}

	encoded, err := json.Marshal(deck)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	payload := string(encoded)
	for _, unwanted := range []string{"g_param", "p_param", "height", "width", "can_edit", "deck_id", "slot", "type", "rare", "card_kind", "affiliation", "num"} {
		if strings.Contains(payload, `"`+unwanted+`"`) {
			t.Fatalf("normalized deck still contains %q: %s", unwanted, payload)
		}
	}
}
