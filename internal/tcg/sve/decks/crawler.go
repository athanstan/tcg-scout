package decks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
	batch, err := client.FetchDecks(ctx, deckIDs, cfg.DeckConcurrency)
	if err != nil {
		return nil, RunSummary{}, err
	}

	failures := append([]DeckFetchFailure(nil), batch.Failures...)
	scraped := len(batch.Decks)
	for _, failure := range failures {
		slog.Warn("deck fetch failed", "deck_id", failure.DeckID, "reason", failure.Reason)
	}

	page.Meta.CrawledAt = time.Now().UTC()
	page.Meta.DecksTotal = len(deckIDs)
	page.Meta.DecksScraped = scraped
	page.Meta.DecksFailed = len(failures)

	outputPath, err := outputPathFor(cfg, page.Meta.Slug)
	if err != nil {
		return nil, RunSummary{}, err
	}

	var payload json.RawMessage
	switch page.Format {
	case FormatSolo:
		entries := append([]SoloEntry(nil), page.Solo...)
		for i := range entries {
			entries[i].TournamentParticipants = page.Meta.EntryCount
			deckID := entries[i].DeckID
			raw, ok := batch.Decks[deckID]
			if !ok {
				entries[i].DeckFetchError = failureReason(failures, deckID)
				continue
			}
			deck, err := NormalizeDeck(raw)
			if err != nil {
				reason := fmt.Sprintf("normalize deck: %v", err)
				slog.Warn("deck normalize failed", "deck_id", deckID, "reason", reason)
				entries[i].DeckFetchError = reason
				failures = appendFailure(failures, deckID, reason)
				scraped--
				page.Meta.DecksScraped = scraped
				page.Meta.DecksFailed = len(failures)
				continue
			}
			entries[i].Deck = deck
		}
		file, buildErr := buildTournamentFile(page.Meta, entries, failures)
		if buildErr != nil {
			return nil, RunSummary{}, buildErr
		}
		payload, err = marshalPayload(file)
	case FormatTeam:
		teams := append([]Team(nil), page.Teams...)
		for i := range teams {
			for j := range teams[i].Decks {
				deckID := teams[i].Decks[j].DeckID
				raw, ok := batch.Decks[deckID]
				if !ok {
					teams[i].Decks[j].DeckFetchError = failureReason(failures, deckID)
					continue
				}
				deck, err := NormalizeDeck(raw)
				if err != nil {
					reason := fmt.Sprintf("normalize deck: %v", err)
					slog.Warn("deck normalize failed", "deck_id", deckID, "reason", reason)
					teams[i].Decks[j].DeckFetchError = reason
					failures = appendFailure(failures, deckID, reason)
					scraped--
					page.Meta.DecksScraped = scraped
					page.Meta.DecksFailed = len(failures)
					continue
				}
				teams[i].Decks[j].Deck = deck
			}
		}
		file, buildErr := buildTournamentFile(page.Meta, teams, failures)
		if buildErr != nil {
			return nil, RunSummary{}, buildErr
		}
		payload, err = marshalPayload(file)
	default:
		return nil, RunSummary{}, fmt.Errorf("unsupported tournament format %q", page.Format)
	}
	if err != nil {
		return nil, RunSummary{}, err
	}

	if scraped == 0 {
		return nil, RunSummary{}, fmt.Errorf("no decks scraped (%d failed)", len(failures))
	}

	summary := RunSummary{
		Slug:           page.Meta.Slug,
		Format:         page.Format,
		DecksTotal:     len(deckIDs),
		DecksScraped:   scraped,
		DecksFailed:    len(failures),
		OutputJSONPath: outputPath,
	}
	return payload, summary, nil
}

func appendFailure(failures []DeckFetchFailure, deckID, reason string) []DeckFetchFailure {
	for _, failure := range failures {
		if failure.DeckID == deckID {
			return failures
		}
	}
	return append(failures, DeckFetchFailure{DeckID: deckID, Reason: reason})
}

func buildTournamentFile(meta TournamentMeta, decks any, failures []DeckFetchFailure) (TournamentFile, error) {
	deckData, err := json.Marshal(decks)
	if err != nil {
		return TournamentFile{}, fmt.Errorf("marshal decks: %w", err)
	}

	file := TournamentFile{
		Title:      meta.Name,
		Link:       meta.SourceURL,
		Date:       meta.Date,
		Identifier: meta.Slug,
		Tournament: TournamentInfo{
			Format:       meta.Format,
			Location:     meta.Location,
			EntryCount:   meta.EntryCount,
			TeamCount:    meta.TeamCount,
			DecksTotal:   meta.DecksTotal,
			DecksScraped: meta.DecksScraped,
			DecksFailed:  meta.DecksFailed,
			CrawledAt:    meta.CrawledAt,
		},
		Decks: deckData,
	}
	if len(failures) > 0 {
		file.FailedDecks = failures
	}
	return file, nil
}

func failureReason(failures []DeckFetchFailure, deckID string) string {
	for _, failure := range failures {
		if failure.DeckID == deckID {
			return failure.Reason
		}
	}
	return "deck payload unavailable"
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

	if err := os.WriteFile(path, append(payload, '\n'), 0o644); err != nil {
		return fmt.Errorf("write tournament json: %w", err)
	}
	return nil
}
