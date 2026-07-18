package decks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSoloTournamentPage(t *testing.T) {
	t.Parallel()

	html := readFixture(t, "bcs2526-sydney.html")
	page, err := ParseTournamentPage("https://en.shadowverse-evolve.com/decks/bcs2526-sydney/", html)
	if err != nil {
		t.Fatalf("ParseTournamentPage() error = %v", err)
	}

	if page.Format != FormatSolo {
		t.Fatalf("Format = %q, want %q", page.Format, FormatSolo)
	}
	if len(page.Solo) != 32 {
		t.Fatalf("len(Solo) = %d, want 32", len(page.Solo))
	}
	if page.Meta.Slug != "bcs2526-sydney" {
		t.Fatalf("Slug = %q", page.Meta.Slug)
	}
	if page.Meta.Name != "Bushiroad Championship Series 25/26 Event Report—Sydney, Australia" {
		t.Fatalf("Name = %q", page.Meta.Name)
	}
	if page.Meta.Date != "Apr. 6, 2026" {
		t.Fatalf("Date = %q", page.Meta.Date)
	}
	if page.Meta.Location != "Sydney, Australia" {
		t.Fatalf("Location = %q", page.Meta.Location)
	}

	first := page.Solo[0]
	if first.DeckID != "5RC61" || first.Player != "JasonZ" || first.Wins != 7 {
		t.Fatalf("first entry = %#v", first)
	}
	if first.WinScope != WinScopePlayer {
		t.Fatalf("WinScope = %q", first.WinScope)
	}
	if first.StandingLabel != "1st place · 7 wins" {
		t.Fatalf("StandingLabel = %q", first.StandingLabel)
	}
}

func TestParseTeamTournamentPage(t *testing.T) {
	t.Parallel()

	html := readFixture(t, "bsf2026-philippines.html")
	page, err := ParseTournamentPage("https://en.shadowverse-evolve.com/decks/bsf2026-philippines/", html)
	if err != nil {
		t.Fatalf("ParseTournamentPage() error = %v", err)
	}

	if page.Format != FormatTeam {
		t.Fatalf("Format = %q, want %q", page.Format, FormatTeam)
	}
	if page.Meta.TeamCount != 8 {
		t.Fatalf("TeamCount = %d, want 8", page.Meta.TeamCount)
	}
	if page.Meta.Date != "Jul. 6, 2026" {
		t.Fatalf("Date = %q", page.Meta.Date)
	}

	deckCount := 0
	for _, team := range page.Teams {
		deckCount += len(team.Decks)
	}
	if deckCount != 24 {
		t.Fatalf("deck count = %d, want 24", deckCount)
	}

	firstTeam := page.Teams[0]
	if firstTeam.TeamName != "UMR" || firstTeam.TeamWins != 4 {
		t.Fatalf("first team = %#v", firstTeam)
	}
	if firstTeam.StandingLabel != "1st place team · 4 team wins" {
		t.Fatalf("StandingLabel = %q", firstTeam.StandingLabel)
	}
	if len(firstTeam.Decks) != 3 || firstTeam.Decks[0].Player != "Soar" {
		t.Fatalf("first team decks = %#v", firstTeam.Decks)
	}

	compactTeam := page.Teams[3]
	if compactTeam.TeamName != "Bingo Plus" || compactTeam.TeamWins != 2 {
		t.Fatalf("compact team = %#v", compactTeam)
	}
	if len(compactTeam.Decks) != 3 {
		t.Fatalf("compact team deck count = %d", len(compactTeam.Decks))
	}

	ids := collectDeckIDs(page)
	if len(ids) != 24 {
		t.Fatalf("collectDeckIDs() = %d, want 24", len(ids))
	}
	for _, crossCraftID := range []string{"59LKZ", "2NC2P", "6CLRR"} {
		for _, id := range ids {
			if id == crossCraftID {
				t.Fatalf("cross craft deck id %s should not be parsed", crossCraftID)
			}
		}
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}
