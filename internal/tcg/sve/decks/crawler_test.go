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
		_, _ = w.Write([]byte(`{"deck_id":"` + deckID + `","title":"fixture","list":[]}`))
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

	if summary.DeckCount != 32 {
		t.Fatalf("DeckCount = %d, want 32", summary.DeckCount)
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

	var file SoloTournamentFile
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(file.Entries) != 32 {
		t.Fatalf("len(entries) = %d", len(file.Entries))
	}
	if file.Entries[0].Deck == nil {
		t.Fatal("expected deck payload on first entry")
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
		_, _ = w.Write([]byte(`{"deck_id":"` + deckID + `","title":"fixture","list":[]}`))
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

	if summary.DeckCount != 24 {
		t.Fatalf("DeckCount = %d, want 24", summary.DeckCount)
	}
	if summary.Format != FormatTeam {
		t.Fatalf("Format = %q", summary.Format)
	}

	data, err := os.ReadFile(summary.OutputJSONPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var file TeamTournamentFile
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if file.Tournament.TeamCount != 8 {
		t.Fatalf("TeamCount = %d", file.Tournament.TeamCount)
	}
	if len(file.Teams) != 8 {
		t.Fatalf("len(teams) = %d", len(file.Teams))
	}
	if file.Teams[0].StandingLabel != "1st place team · 4 team wins" {
		t.Fatalf("StandingLabel = %q", file.Teams[0].StandingLabel)
	}
}
