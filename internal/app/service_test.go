package app

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeRunner struct {
	definition RunnerDefinition
	result     Result
	err        error
}

func (f fakeRunner) Definition() RunnerDefinition {
	return f.definition
}

func (f fakeRunner) Execute(_ context.Context, _ string, _ Request) (Result, error) {
	if f.err != nil {
		return Result{}, f.err
	}
	return f.result, nil
}

func TestNewService(t *testing.T) {
	tests := []struct {
		name    string
		runners []Runner
		wantErr string
	}{
		{
			name: "builds catalog",
			runners: []Runner{
				fakeRunner{definition: RunnerDefinition{
					GameID:          "beta",
					GameName:        "Beta",
					GameSummary:     "Second",
					ResourceID:      "decks",
					ResourceName:    "Decks",
					ResourceSummary: "Deck workflows",
					ScraperID:       "community",
					ScraperName:     "Community",
					ScraperSummary:  "Community scraper",
					DefaultAction:   ActionList,
					Actions:         []ActionDefinition{{ID: ActionList, Summary: "List"}},
				}},
				fakeRunner{definition: RunnerDefinition{
					GameID:          "alpha",
					GameName:        "Alpha",
					GameSummary:     "First",
					ResourceID:      "cards",
					ResourceName:    "Cards",
					ResourceSummary: "Card workflows",
					ScraperID:       "official",
					ScraperName:     "Official",
					ScraperSummary:  "Official scraper",
					DefaultAction:   ActionScrape,
					Actions:         []ActionDefinition{{ID: ActionScrape, Summary: "Scrape"}},
				}},
			},
		},
		{
			name: "rejects duplicate scraper",
			runners: []Runner{
				fakeRunner{definition: RunnerDefinition{
					GameID:     "alpha",
					ResourceID: "cards",
					ScraperID:  "official",
				}},
				fakeRunner{definition: RunnerDefinition{
					GameID:     "alpha",
					ResourceID: "cards",
					ScraperID:  "official",
				}},
			},
			wantErr: "duplicate scraper",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, err := NewService(tt.runners...)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("NewService() error = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewService() error = %v", err)
			}

			games := service.Games()
			if len(games) != 2 {
				t.Fatalf("len(Games()) = %d, want 2", len(games))
			}
			if games[0].ID != "alpha" || games[1].ID != "beta" {
				t.Fatalf("Games() order = %#v", games)
			}
		})
	}
}

func TestServiceExecute(t *testing.T) {
	t.Run("fills selection details into summary", func(t *testing.T) {
		t.Parallel()

		service, err := NewService(fakeRunner{
			definition: RunnerDefinition{
				GameID:          "alpha",
				GameName:        "Alpha",
				GameSummary:     "First",
				ResourceID:      "cards",
				ResourceName:    "Cards",
				ResourceSummary: "Card workflows",
				ScraperID:       "official",
				ScraperName:     "Official",
				ScraperSummary:  "Official scraper",
				DefaultAction:   ActionList,
				Actions:         []ActionDefinition{{ID: ActionList, Summary: "List"}},
			},
			result: Result{Summary: Summary{Message: "done"}},
		})
		if err != nil {
			t.Fatalf("NewService() error = %v", err)
		}

		result, err := service.Execute(context.Background(), Selection{
			Game:     "alpha",
			Resource: "cards",
			Scraper:  "official",
		}, ActionList, Request{})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if result.Summary.Game != "alpha" || result.Summary.Resource != "cards" || result.Summary.Scraper != "official" || result.Summary.Action != ActionList {
			t.Fatalf("summary = %#v", result.Summary)
		}
	})

	tests := []struct {
		name      string
		selection Selection
		action    string
		wantErr   string
	}{
		{
			name:      "unknown game",
			selection: Selection{Game: "missing", Resource: "cards", Scraper: "official"},
			action:    ActionList,
			wantErr:   `unknown game "missing"`,
		},
		{
			name:      "unknown resource",
			selection: Selection{Game: "alpha", Resource: "missing", Scraper: "official"},
			action:    ActionList,
			wantErr:   `unknown resource "missing" for game "alpha"`,
		},
		{
			name:      "unknown scraper",
			selection: Selection{Game: "alpha", Resource: "cards", Scraper: "missing"},
			action:    ActionList,
			wantErr:   `unknown scraper "missing" for alpha cards`,
		},
		{
			name:      "unknown action",
			selection: Selection{Game: "alpha", Resource: "cards", Scraper: "official"},
			action:    ActionImages,
			wantErr:   `unknown action "images" for alpha cards official`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			service, err := NewService(fakeRunner{
				definition: RunnerDefinition{
					GameID:          "alpha",
					GameName:        "Alpha",
					GameSummary:     "First",
					ResourceID:      "cards",
					ResourceName:    "Cards",
					ResourceSummary: "Card workflows",
					ScraperID:       "official",
					ScraperName:     "Official",
					ScraperSummary:  "Official scraper",
					DefaultAction:   ActionList,
					Actions:         []ActionDefinition{{ID: ActionList, Summary: "List"}},
				},
				err: errors.New("boom"),
			})
			if err != nil {
				t.Fatalf("NewService() error = %v", err)
			}

			_, err = service.Execute(context.Background(), tt.selection, tt.action, Request{})
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("Execute() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}
