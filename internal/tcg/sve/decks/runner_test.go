package decks

import (
	"context"
	"strings"
	"testing"

	"tcg-scout/internal/app"
)

func TestRunnerDefinition(t *testing.T) {
	t.Parallel()

	definition := NewRunner().Definition()
	if definition.GameID != "sve" || definition.ResourceID != "decks" || definition.ScraperID != "official" {
		t.Fatalf("Definition() = %#v", definition)
	}
	if len(definition.Actions) != 2 {
		t.Fatalf("len(Definition().Actions) = %d, want 2", len(definition.Actions))
	}
}

func TestRunnerExecuteUnsupportedAction(t *testing.T) {
	t.Parallel()

	_, err := NewRunner().Execute(context.Background(), "unknown", app.DefaultRequest())
	if err == nil || !strings.Contains(err.Error(), `unsupported action "unknown"`) {
		t.Fatalf("Execute() error = %v", err)
	}
}
