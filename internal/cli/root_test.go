package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"tcg-scout/internal/app"
	"tcg-scout/internal/scraper"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

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
			name:            "invalid interactive choice",
			input:           "9\n",
			args:            []string{"interactive", "--output", "plain"},
			wantErrContains: `invalid choice "9"`,
		},
		{
			name:            "unknown resource",
			args:            []string{"resources", "missing"},
			wantErrContains: `unknown game "missing"`,
		},
		{
			name:            "usage errors map to exit code two",
			args:            []string{"unknown"},
			wantErrContains: `unknown command "unknown"`,
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
				if tt.name == "usage errors map to exit code two" && ExitCode(err) != 2 {
					t.Fatalf("ExitCode() = %d, want 2", ExitCode(err))
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
	service, err := app.NewService(fakeRunner{})
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
