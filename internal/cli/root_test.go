package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"tcg-scout/internal/app"
	"tcg-scout/internal/scraper"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "nil", err: nil, want: 0},
		{name: "usage error", err: usageErrorf("unknown game %q", "missing"), want: 2},
		{name: "runtime error", err: errors.New("fetch failed"), want: 1},
		{name: "cobra unknown command", err: errors.New(`unknown command "foo" for "tcg-scout"`), want: 2},
		{name: "cobra required flag", err: errors.New(`required flag(s) "tournament-url" not set`), want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExitCode(tt.err); got != tt.want {
				t.Fatalf("ExitCode() = %d, want %d", got, tt.want)
			}
		})
	}
}

type fakeRunner struct{}

func (fakeRunner) Definition() app.RunnerDefinition {
	return app.RunnerDefinition{
		GameID:          "demo",
		GameName:        "Demo Game",
		GameSummary:     "Demo summary",
		ResourceID:      "cards",
		ResourceName:    "Cards",
		ResourceSummary: "Card workflows",
		ScraperID:       "alpha",
		ScraperName:     "Alpha Scraper",
		ScraperSummary:  "Primary scraper",
		DefaultAction:   app.ActionList,
		Actions: []app.ActionDefinition{
			{ID: app.ActionList, Summary: "List cards"},
			{ID: app.ActionScrape, Summary: "Scrape cards"},
		},
	}
}

func (fakeRunner) Execute(_ context.Context, action string, _ app.Request) (app.Result, error) {
	result := app.Result{
		Summary: app.Summary{
			Game:      "demo",
			Resource:  "cards",
			Scraper:   "alpha",
			Action:    action,
			CardCount: 1,
			Message:   "done",
		},
	}
	if action == app.ActionList {
		result.Cards = []scraper.Card{{ID: "C-001", Name: "Alpha Card", Rarity: "Legendary", MainType: "Follower", SetID: "BP17"}}
	}
	return result, nil
}

type fakeDecksRunner struct{}

func (fakeDecksRunner) Definition() app.RunnerDefinition {
	return app.RunnerDefinition{
		GameID:          "demo",
		GameName:        "Demo Game",
		GameSummary:     "Demo summary",
		ResourceID:      "decks",
		ResourceName:    "Decks",
		ResourceSummary: "Deck workflows",
		ScraperID:       "official",
		ScraperName:     "Official site",
		ScraperSummary:  "Official deck scraper",
		DefaultAction:   app.ActionScrape,
		Actions: []app.ActionDefinition{
			{ID: app.ActionScrape, Summary: "Scrape decks"},
			{ID: app.ActionList, Summary: "List decks"},
		},
	}
}

func (fakeDecksRunner) Execute(_ context.Context, action string, req app.Request) (app.Result, error) {
	if strings.TrimSpace(req.TournamentURL) == "" {
		return app.Result{}, fmt.Errorf("tournament url is required")
	}
	return app.Result{
		Summary: app.Summary{
			Game:     "demo",
			Resource: "decks",
			Scraper:  "official",
			Action:   action,
			Message:  "deck done for " + req.TournamentURL,
		},
	}, nil
}

func TestRewriteArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "legacy names only",
			args: []string{"-names-only", "-search-url", "https://example.com"},
			want: []string{"sve", "cards", "official", "list", "-search-url", "https://example.com"},
		},
		{
			name: "images only alias",
			args: []string{"images-only", "--output", "json"},
			want: []string{"sve", "cards", "official", "images", "--output", "json"},
		},
		{
			name: "legacy scrape flags",
			args: []string{"--output", "json"},
			want: []string{"sve", "cards", "official", "scrape", "--output", "json"},
		},
		{
			name: "legacy explicit action without scraper",
			args: []string{"sve", "cards", "scrape", "--output", "json"},
			want: []string{"sve", "cards", "official", "scrape", "--output", "json"},
		},
		{
			name: "decks scrape shorthand",
			args: []string{"sve", "decks", "scrape", "--tournament-url", "https://example.com/decks/test/"},
			want: []string{"sve", "decks", "official", "scrape", "--tournament-url", "https://example.com/decks/test/"},
		},
		{
			name: "decks flags only",
			args: []string{"sve", "decks", "--tournament-url", "https://example.com/decks/test/"},
			want: []string{"sve", "decks", "official", "scrape", "--tournament-url", "https://example.com/decks/test/"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RewriteArgs(tt.args)
			if strings.Join(got, " ") != strings.Join(tt.want, " ") {
				t.Fatalf("RewriteArgs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestRootCommand(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		args            []string
		wantContains    []string
		wantErrContains string
		wantExitCode    int
	}{
		{
			name:         "games json",
			args:         []string{"games", "--output", "json"},
			wantContains: []string{`"demo"`, `"Demo Game"`},
		},
		{
			name:         "resources plain",
			args:         []string{"resources", "demo", "--output", "plain"},
			wantContains: []string{"cards\tCards"},
		},
		{
			name:         "list cards as json",
			args:         []string{"demo", "cards", "alpha", "list", "--output", "json"},
			wantContains: []string{`"Alpha Card"`, `"cards"`},
		},
		{
			name:         "version command",
			args:         []string{"version"},
			wantContains: []string{"tcg-scout dev"},
		},
		{
			name:         "interactive flow",
			input:        "1\n1\n1\n1\n",
			args:         []string{"interactive", "--output", "plain"},
			wantContains: []string{"Choose a game", "C-001\tAlpha Card"},
		},
		{
			name:         "interactive decks prompts for tournament url",
			input:        "1\n2\n1\n1\nhttps://example.com/decks/test/\n",
			args:         []string{"interactive", "--output", "plain"},
			wantContains: []string{"Enter tournament deck list URL", "deck done for https://example.com/decks/test/"},
		},
		{
			name:         "scraper level inherits resource flags",
			args:         []string{"demo", "cards", "alpha", "list", "--search-url", "https://example.com/cards", "--output", "json"},
			wantContains: []string{`"Alpha Card"`},
		},
		{
			name:            "rejects unknown positional arg on resource",
			args:            []string{"demo", "decks", "foo"},
			wantErrContains: `unknown command "foo" for "tcg-scout demo decks"`,
			wantExitCode:    2,
		},
		{
			name:            "requires tournament url for deck scrape",
			args:            []string{"demo", "decks"},
			wantErrContains: `required flag(s) "tournament-url" not set`,
			wantExitCode:    2,
		},
		{
			name:            "invalid interactive choice",
			input:           "9\n",
			args:            []string{"interactive", "--output", "plain"},
			wantErrContains: `invalid choice "9"`,
			wantExitCode:    2,
		},
		{
			name:            "unknown resource",
			args:            []string{"resources", "missing"},
			wantErrContains: `unknown game "missing"`,
			wantExitCode:    2,
		},
		{
			name:            "usage errors map to exit code two",
			args:            []string{"unknown"},
			wantErrContains: `unknown command "unknown"`,
			wantExitCode:    2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, stdout, _ := newTestRoot(t, tt.input)
			root.SetArgs(tt.args)

			err := root.Execute()
			if tt.wantErrContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf("Execute() error = %v, want substring %q", err, tt.wantErrContains)
				}
				if tt.wantExitCode != 0 && ExitCode(err) != tt.wantExitCode {
					t.Fatalf("ExitCode() = %d, want %d", ExitCode(err), tt.wantExitCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}

			output := stdout.String()
			for _, want := range tt.wantContains {
				if !strings.Contains(output, want) {
					t.Fatalf("output = %s, missing %q", output, want)
				}
			}
		})
	}
}

func newTestRoot(t *testing.T, input string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	viper.Reset()
	service, err := app.NewService(fakeRunner{}, fakeDecksRunner{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root := NewRootCommand(service, Streams{
		In:  strings.NewReader(input),
		Out: &stdout,
		Err: &stderr,
	}, BuildInfo{})
	return root, &stdout, &stderr
}
