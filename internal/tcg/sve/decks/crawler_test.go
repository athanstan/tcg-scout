package decks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScrapeSoloTournament(t *testing.T) {
	t.Parallel()

	html := readFixture(t, "bcs2526-sydney.html")
	pageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(html)
	}))
	defer pageServer.Close()

	decklogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deckID := strings.TrimPrefix(r.URL.Path, "/system/app/api/view/")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"deck_id":"` + deckID + `","title":"fixture","deck_param2":"Dragoncraft","list":[{"card_number":"BP17-001EN","name":"Arisa","num":3,"cost":"1","rare":"Legendary","card_kind":"Follower","affiliation":"Dragoncraft","img":"BP17/BP17-001EN.png"}]}`))
	}))
	defer decklogServer.Close()

	testClient := &http.Client{Timeout: 30 * time.Second}

	outputDir := t.TempDir()
	summary, err := Scrape(context.Background(), Config{
		TournamentURL:    pageServer.URL + "/decks/bcs2526-sydney/",
		OutputDir:        outputDir,
		UserAgent:        "tcg-scout-test",
		DeckConcurrency:  4,
		HTTPClient:       testClient,
		DecklogAPIBase:   decklogServer.URL + "/system/app/api/view/",
	})
	if err != nil {
		t.Fatalf("Scrape() error = %v", err)
	}

	if summary.DecksTotal != 32 {
		t.Fatalf("DecksTotal = %d, want 32", summary.DecksTotal)
	}
	if summary.DecksScraped != 32 {
		t.Fatalf("DecksScraped = %d, want 32", summary.DecksScraped)
	}
	if summary.Format != FormatSolo {
		t.Fatalf("Format = %q", summary.Format)
	}

	outputPath := filepath.Join(outputDir, "bcs2526-sydney-decks.json")
	if summary.OutputJSONPath != outputPath {
		t.Fatalf("OutputJSONPath = %q, want %q", summary.OutputJSONPath, outputPath)
	}
	if _, err := os.Stat(summary.OutputJSONPath); err != nil {
		t.Fatalf("output file missing: %v", err)
	}

	data, err := os.ReadFile(summary.OutputJSONPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var file TournamentFile
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if file.Identifier != "bcs2526-sydney" {
		t.Fatalf("Identifier = %q", file.Identifier)
	}
	if file.Title == "" {
		t.Fatal("expected tournament title")
	}
	if file.Date != "Apr. 6, 2026" {
		t.Fatalf("Date = %q", file.Date)
	}
	if file.Link == "" {
		t.Fatal("expected tournament link")
	}
	if file.Tournament.Format != FormatSolo {
		t.Fatalf("Format = %q", file.Tournament.Format)
	}
	var soloEntries []SoloEntry
	if err := json.Unmarshal(file.Decks, &soloEntries); err != nil {
		t.Fatalf("Unmarshal decks: %v", err)
	}
	if len(soloEntries) != 32 {
		t.Fatalf("len(decks) = %d", len(soloEntries))
	}
	if soloEntries[0].Deck.Title == "" {
		t.Fatal("expected normalized deck title on first entry")
	}
	if soloEntries[0].TournamentParticipants != 32 {
		t.Fatalf("TournamentParticipants = %d, want 32", soloEntries[0].TournamentParticipants)
	}
	if soloEntries[0].Ranking != 1 {
		t.Fatalf("Ranking = %d, want 1", soloEntries[0].Ranking)
	}
	if len(soloEntries[0].Deck.Cards) == 0 {
		t.Fatal("expected cards on normalized deck")
	}
}

func TestScrapeTeamTournament(t *testing.T) {
	t.Parallel()

	html := readFixture(t, "bsf2026-philippines.html")
	pageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(html)
	}))
	defer pageServer.Close()

	decklogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deckID := strings.TrimPrefix(r.URL.Path, "/system/app/api/view/")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"deck_id":"` + deckID + `","title":"fixture","deck_param2":"Dragoncraft","list":[{"card_number":"BP17-001EN","name":"Arisa","num":3,"cost":"1","rare":"Legendary","card_kind":"Follower","affiliation":"Dragoncraft","img":"BP17/BP17-001EN.png"}]}`))
	}))
	defer decklogServer.Close()

	testClient := &http.Client{Timeout: 30 * time.Second}

	outputDir := t.TempDir()
	summary, err := Scrape(context.Background(), Config{
		TournamentURL:    pageServer.URL + "/decks/bsf2026-philippines/",
		OutputDir:        outputDir,
		UserAgent:        "tcg-scout-test",
		DeckConcurrency:  4,
		HTTPClient:       testClient,
		DecklogAPIBase:   decklogServer.URL + "/system/app/api/view/",
	})
	if err != nil {
		t.Fatalf("Scrape() error = %v", err)
	}

	if summary.DecksTotal != 24 {
		t.Fatalf("DecksTotal = %d, want 24", summary.DecksTotal)
	}
	if summary.DecksScraped != 24 {
		t.Fatalf("DecksScraped = %d, want 24", summary.DecksScraped)
	}
	if summary.Format != FormatTeam {
		t.Fatalf("Format = %q", summary.Format)
	}

	data, err := os.ReadFile(summary.OutputJSONPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var file TournamentFile
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if file.Identifier != "bsf2026-philippines" {
		t.Fatalf("Identifier = %q", file.Identifier)
	}
	if file.Tournament.TeamCount != 8 {
		t.Fatalf("TeamCount = %d", file.Tournament.TeamCount)
	}
	var teams []Team
	if err := json.Unmarshal(file.Decks, &teams); err != nil {
		t.Fatalf("Unmarshal decks: %v", err)
	}
	if len(teams) != 8 {
		t.Fatalf("len(decks) = %d", len(teams))
	}
	if teams[0].StandingLabel != "1st place team · 4 team wins" {
		t.Fatalf("StandingLabel = %q", teams[0].StandingLabel)
	}
}

func TestScrapePartialDeckFailures(t *testing.T) {
	t.Parallel()

	html := readFixture(t, "bcs2526-sydney.html")
	pageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(html)
	}))
	defer pageServer.Close()

	decklogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deckID := strings.TrimPrefix(r.URL.Path, "/system/app/api/view/")
		w.Header().Set("Content-Type", "application/json")
		if deckID == "5RC61" {
			_, _ = w.Write([]byte("[]"))
			return
		}
		_, _ = w.Write([]byte(`{"deck_id":"` + deckID + `","title":"fixture","deck_param2":"Dragoncraft","list":[{"card_number":"BP17-001EN","name":"Arisa","num":3,"cost":"1","rare":"Legendary","card_kind":"Follower","affiliation":"Dragoncraft","img":"BP17/BP17-001EN.png"}]}`))
	}))
	defer decklogServer.Close()

	outputDir := t.TempDir()
	summary, err := Scrape(context.Background(), Config{
		TournamentURL:   pageServer.URL + "/decks/bcs2526-sydney/",
		OutputDir:       outputDir,
		UserAgent:       "tcg-scout-test",
		DeckConcurrency: 4,
		HTTPClient:      &http.Client{Timeout: 30 * time.Second},
		DecklogAPIBase:  decklogServer.URL + "/system/app/api/view/",
	})
	if err != nil {
		t.Fatalf("Scrape() error = %v", err)
	}
	if summary.DecksTotal != 32 {
		t.Fatalf("DecksTotal = %d, want 32", summary.DecksTotal)
	}
	if summary.DecksScraped != 31 {
		t.Fatalf("DecksScraped = %d, want 31", summary.DecksScraped)
	}
	if summary.DecksFailed != 1 {
		t.Fatalf("DecksFailed = %d, want 1", summary.DecksFailed)
	}

	data, err := os.ReadFile(summary.OutputJSONPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var file TournamentFile
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if file.Tournament.DecksScraped != 31 || file.Tournament.DecksFailed != 1 {
		t.Fatalf("tournament counts = scraped %d failed %d", file.Tournament.DecksScraped, file.Tournament.DecksFailed)
	}
	if len(file.FailedDecks) != 1 || file.FailedDecks[0].DeckID != "5RC61" {
		t.Fatalf("FailedDecks = %#v", file.FailedDecks)
	}
	var soloEntries []SoloEntry
	if err := json.Unmarshal(file.Decks, &soloEntries); err != nil {
		t.Fatalf("Unmarshal decks: %v", err)
	}
	if soloEntries[0].DeckFetchError != "empty decklog response" {
		t.Fatalf("DeckFetchError = %q", soloEntries[0].DeckFetchError)
	}
	if soloEntries[0].Deck.Title != "" {
		t.Fatal("expected missing deck payload on failed entry")
	}
	if soloEntries[1].Deck.Title == "" {
		t.Fatal("expected deck payload on successful entry")
	}
}
