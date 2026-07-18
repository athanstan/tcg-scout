package decks

import (
	"context"
	"fmt"

	"tcg-scout/internal/app"
)

type Runner struct{}

func NewRunner() Runner {
	return Runner{}
}

func (Runner) Definition() app.RunnerDefinition {
	return app.RunnerDefinition{
		GameID:          "sve",
		GameName:        "Shadowverse Evolve",
		GameSummary:     "English Shadowverse Evolve scraping workflows.",
		ResourceID:      "decks",
		ResourceName:    "Decks",
		ResourceSummary: "Crawl official tournament deck lists from Shadowverse Evolve event reports.",
		ScraperID:       "official",
		ScraperName:     "Official site",
		ScraperSummary:  "Parses tournament pages for deck IDs and fetches deck payloads from Decklog.",
		DefaultAction:   app.ActionScrape,
		Actions: []app.ActionDefinition{
			{ID: app.ActionScrape, Summary: "Fetch tournament decks and write {slug}-decks.json."},
			{ID: app.ActionList, Summary: "Fetch tournament decks and return JSON without writing files."},
		},
	}
}

func (r Runner) Execute(ctx context.Context, action string, req app.Request) (app.Result, error) {
	cfg := Config{
		TournamentURL:   req.TournamentURL,
		OutputDir:       req.OutputDecksDir,
		UserAgent:       req.UserAgent,
		DeckConcurrency: req.DeckConcurrency,
	}

	switch action {
	case app.ActionScrape:
		summary, err := Scrape(ctx, cfg)
		if err != nil {
			return app.Result{}, err
		}
		return app.Result{
			Summary: app.Summary{
				DeckCount:      summary.DecksScraped,
				DecksTotal:     summary.DecksTotal,
				DecksFailed:    summary.DecksFailed,
				OutputJSONPath: summary.OutputJSONPath,
				Format:         summary.Format,
				Message:        formatDeckSummary("scraped", summary),
			},
		}, nil
	case app.ActionList:
		payload, summary, err := List(ctx, cfg)
		if err != nil {
			return app.Result{}, err
		}
		return app.Result{
			Tournament: payload,
			Summary: app.Summary{
				DeckCount:   summary.DecksScraped,
				DecksTotal:  summary.DecksTotal,
				DecksFailed: summary.DecksFailed,
				Format:      summary.Format,
				Message:     formatDeckSummary("listed", summary),
			},
		}, nil
	default:
		return app.Result{}, fmt.Errorf("unsupported action %q", action)
	}
}

func formatDeckSummary(verb string, summary RunSummary) string {
	message := fmt.Sprintf("%s %d/%d decks for %s (%s)", verb, summary.DecksScraped, summary.DecksTotal, summary.Slug, summary.Format)
	if summary.DecksFailed > 0 {
		message += fmt.Sprintf("; %d failed", summary.DecksFailed)
	}
	return message
}
