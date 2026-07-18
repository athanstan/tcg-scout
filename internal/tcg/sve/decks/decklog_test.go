package decks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDecklogClientFetchDeck(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != DecklogReferer {
			t.Fatalf("Referer = %q", r.Header.Get("Referer"))
		}
		if r.URL.Path != "/system/app/api/view/ABC12" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"deck_id":"ABC12","title":"Test Deck","list":[]}`))
	}))
	defer server.Close()

	client := NewDecklogClient(Config{
		UserAgent:      "tcg-scout-test",
		HTTPClient:     server.Client(),
		DecklogAPIBase: server.URL + "/system/app/api/view/",
	})

	payload, err := client.FetchDeck(context.Background(), "ABC12")
	if err != nil {
		t.Fatalf("FetchDeck() error = %v", err)
	}
	if !json.Valid(payload) {
		t.Fatalf("invalid json payload: %s", payload)
	}
}

func TestDecklogClientFetchDeckEmptyResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("[]"))
	}))
	defer server.Close()

	client := NewDecklogClient(Config{
		UserAgent:      "tcg-scout-test",
		HTTPClient:     server.Client(),
		DecklogAPIBase: server.URL + "/system/app/api/view/",
	})

	_, err := client.FetchDeck(context.Background(), "EMPTY")
	if err == nil {
		t.Fatal("expected error for empty deck response")
	}
}

func TestDecklogClientFetchDecks(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deckID := r.URL.Path[len("/system/app/api/view/"):]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"deck_id":"` + deckID + `","list":[]}`))
	}))
	defer server.Close()

	client := NewDecklogClient(Config{
		UserAgent:      "tcg-scout-test",
		HTTPClient:     server.Client(),
		DecklogAPIBase: server.URL + "/system/app/api/view/",
	})

	results, err := client.FetchDecks(context.Background(), []string{"A", "B"}, 2)
	if err != nil {
		t.Fatalf("FetchDecks() error = %v", err)
	}
	if len(results.Decks) != 2 {
		t.Fatalf("len(results.Decks) = %d, want 2", len(results.Decks))
	}
}

func TestDecklogClientFetchDecksPartialFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deckID := r.URL.Path[len("/system/app/api/view/"):]
		if deckID == "BAD" {
			_, _ = w.Write([]byte("[]"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"deck_id":"` + deckID + `","list":[]}`))
	}))
	defer server.Close()

	client := NewDecklogClient(Config{
		UserAgent:      "tcg-scout-test",
		HTTPClient:     server.Client(),
		DecklogAPIBase: server.URL + "/system/app/api/view/",
	})

	results, err := client.FetchDecks(context.Background(), []string{"GOOD", "BAD"}, 2)
	if err != nil {
		t.Fatalf("FetchDecks() error = %v", err)
	}
	if len(results.Decks) != 1 {
		t.Fatalf("len(results.Decks) = %d, want 1", len(results.Decks))
	}
	if len(results.Failures) != 1 || results.Failures[0].DeckID != "BAD" {
		t.Fatalf("Failures = %#v", results.Failures)
	}
}
