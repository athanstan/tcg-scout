package decks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func Scrape(ctx context.Context, cfg Config) (RunSummary, error) {
	payload, summary, err := buildTournamentPayload(ctx, cfg)
	if err != nil {
		return RunSummary{}, err
	}

	if err := writeTournamentJSON(summary.OutputJSONPath, payload); err != nil {
		return RunSummary{}, err
	}
	return summary, nil
}

func List(ctx context.Context, cfg Config) (json.RawMessage, RunSummary, error) {
	payload, summary, err := buildTournamentPayload(ctx, cfg)
	if err != nil {
		return nil, RunSummary{}, err
	}
	return payload, summary, nil
}

func buildTournamentPayload(ctx context.Context, cfg Config) (json.RawMessage, RunSummary, error) {
	if strings.TrimSpace(cfg.TournamentURL) == "" {
		return nil, RunSummary{}, fmt.Errorf("tournament url is required")
	}

	pageHTML, err := fetchTournamentPage(ctx, cfg)
	if err != nil {
		return nil, RunSummary{}, err
	}

	page, err := ParseTournamentPage(cfg.TournamentURL, pageHTML)
	if err != nil {
		return nil, RunSummary{}, err
	}

	deckIDs := collectDeckIDs(page)
	if len(deckIDs) == 0 {
		return nil, RunSummary{}, fmt.Errorf("no deck ids found")
	}

	client := NewDecklogClient(cfg)
	decksByID, err := client.FetchDecks(ctx, deckIDs, cfg.DeckConcurrency)
	if err != nil {
		return nil, RunSummary{}, err
	}

	page.Meta.CrawledAt = time.Now().UTC()

	outputPath, err := outputPathFor(cfg, page.Meta.Slug)
	if err != nil {
		return nil, RunSummary{}, err
	}

	var payload json.RawMessage
	switch page.Format {
	case FormatSolo:
		file := SoloTournamentFile{Tournament: page.Meta, Entries: page.Solo}
		for i := range file.Entries {
			deck, ok := decksByID[file.Entries[i].DeckID]
			if !ok {
				return nil, RunSummary{}, fmt.Errorf("missing deck payload for %s", file.Entries[i].DeckID)
			}
			file.Entries[i].Deck = deck
		}
		payload, err = marshalPayload(file)
	case FormatTeam:
		file := TeamTournamentFile{Tournament: page.Meta, Teams: page.Teams}
		for i := range file.Teams {
			for j := range file.Teams[i].Decks {
				deckID := file.Teams[i].Decks[j].DeckID
				deck, ok := decksByID[deckID]
				if !ok {
					return nil, RunSummary{}, fmt.Errorf("missing deck payload for %s", deckID)
				}
				file.Teams[i].Decks[j].Deck = deck
			}
		}
		payload, err = marshalPayload(file)
	default:
		return nil, RunSummary{}, fmt.Errorf("unsupported tournament format %q", page.Format)
	}
	if err != nil {
		return nil, RunSummary{}, err
	}

	summary := RunSummary{
		Slug:           page.Meta.Slug,
		Format:         page.Format,
		DeckCount:      len(deckIDs),
		OutputJSONPath: outputPath,
	}
	return payload, summary, nil
}

func fetchTournamentPage(ctx context.Context, cfg Config) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.TournamentURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build tournament request: %w", err)
	}
	userAgent := cfg.UserAgent
	if userAgent == "" {
		userAgent = DefaultUserAgent
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClientFor(cfg).Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch tournament page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("tournament page: unexpected status %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read tournament page: %w", err)
	}
	return body, nil
}

func outputPathFor(cfg Config, slug string) (string, error) {
	outputDir := strings.TrimSpace(cfg.OutputDir)
	if outputDir == "" {
		outputDir = DefaultOutputDir
	}
	if strings.TrimSpace(slug) == "" {
		return "", fmt.Errorf("tournament slug is required")
	}
	return filepath.Join(outputDir, slug+"-decks.json"), nil
}

func marshalPayload(value any) (json.RawMessage, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal tournament json: %w", err)
	}
	return json.RawMessage(data), nil
}

func writeTournamentJSON(path string, payload json.RawMessage) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")

	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return fmt.Errorf("decode tournament payload: %w", err)
	}
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write tournament json: %w", err)
	}
	return nil
}
