package cards

import (
	"context"
	"fmt"

	"tcg-scout/internal/app"
	"tcg-scout/internal/scraper"
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
		ResourceID:      "cards",
		ResourceName:    "Cards",
		ResourceSummary: "Scrape card metadata and rebuild card art assets.",
		ScraperID:       "official",
		ScraperName:     "Official site",
		ScraperSummary:  "Uses the official English Shadowverse Evolve text search and image endpoints.",
		DefaultAction:   app.ActionScrape,
		Actions: []app.ActionDefinition{
			{ID: app.ActionScrape, Summary: "Fetch card data, write cards.json, and download WEBP images."},
			{ID: app.ActionList, Summary: "Fetch card data and return the current card list without writing files."},
			{ID: app.ActionImages, Summary: "Delete and rebuild WEBP images from an existing cards.json file."},
		},
	}
}

func (Runner) Execute(ctx context.Context, action string, req app.Request) (app.Result, error) {
	cfg := req.ScraperConfig()

	switch action {
	case app.ActionScrape:
		summary, err := scraper.Scrape(ctx, cfg)
		if err != nil {
			return app.Result{}, err
		}
		return app.Result{
			Summary: app.Summary{
				CardCount:      summary.CardCount,
				OutputJSONPath: summary.OutputJSONPath,
				ImagesDir:      summary.ImagesDir,
				Message:        fmt.Sprintf("scraped %d cards and refreshed WEBP images", summary.CardCount),
			},
		}, nil
	case app.ActionList:
		cards, err := scraper.ListCards(ctx, cfg)
		if err != nil {
			return app.Result{}, err
		}
		return app.Result{
			Cards: cards,
			Summary: app.Summary{
				CardCount: len(cards),
				Message:   fmt.Sprintf("listed %d cards", len(cards)),
			},
		}, nil
	case app.ActionImages:
		summary, err := scraper.RebuildImages(ctx, cfg)
		if err != nil {
			return app.Result{}, err
		}
		return app.Result{
			Summary: app.Summary{
				CardCount:      summary.CardCount,
				OutputJSONPath: summary.OutputJSONPath,
				ImagesDir:      summary.ImagesDir,
				Message:        fmt.Sprintf("rebuilt %d WEBP images", summary.CardCount),
			},
		}, nil
	default:
		return app.Result{}, fmt.Errorf("unsupported action %q", action)
	}
}
