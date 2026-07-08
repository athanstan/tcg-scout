package decks

import (
	"encoding/json"
	"net/http"
	"time"
)

const (
	FormatSolo = "solo"
	FormatTeam = "team"

	WinScopePlayer = "player"
	WinScopeTeam   = "team"

	DefaultOutputDir       = "output/decks"
	DefaultDeckConcurrency = 4
	DefaultUserAgent       = "tcg-scout/1.0"
	DecklogViewBase        = "https://decklog-en.bushiroad.com/view/"
	DecklogReferer         = "https://decklog-en.bushiroad.com/"
)

var defaultDecklogAPIBase = "https://decklog-en.bushiroad.com/system/app/api/view/"

type Config struct {
	TournamentURL   string
	OutputDir       string
	UserAgent       string
	DeckConcurrency int
	HTTPClient      *http.Client
	DecklogAPIBase  string
}

type TournamentMeta struct {
	Slug       string    `json:"slug"`
	Name       string    `json:"name"`
	Location   string    `json:"location,omitempty"`
	Format     string    `json:"format"`
	EntryCount int       `json:"entry_count,omitempty"`
	TeamCount  int       `json:"team_count,omitempty"`
	SourceURL  string    `json:"source_url"`
	CrawledAt  time.Time `json:"crawled_at"`
}

type SoloEntry struct {
	Rank          int             `json:"rank"`
	RankLabel     string          `json:"rank_label"`
	Player        string          `json:"player"`
	Craft         string          `json:"craft"`
	Wins          int             `json:"wins"`
	WinScope      string          `json:"win_scope"`
	StandingLabel string          `json:"standing_label"`
	DeckID        string          `json:"deck_id"`
	DecklogURL    string          `json:"decklog_url"`
	Deck          json.RawMessage `json:"deck"`
}

type TeamDeck struct {
	Player     string          `json:"player,omitempty"`
	Craft      string          `json:"craft"`
	DeckID     string          `json:"deck_id"`
	DecklogURL string          `json:"decklog_url"`
	Deck       json.RawMessage `json:"deck"`
}

type Team struct {
	Rank          int        `json:"rank"`
	RankLabel     string     `json:"rank_label"`
	TeamName      string     `json:"team_name"`
	TeamWins      int        `json:"team_wins"`
	WinScope      string     `json:"win_scope"`
	StandingLabel string     `json:"standing_label"`
	Decks         []TeamDeck `json:"decks"`
}

type SoloTournamentFile struct {
	Tournament TournamentMeta `json:"tournament"`
	Entries    []SoloEntry    `json:"entries"`
}

type TeamTournamentFile struct {
	Tournament TournamentMeta `json:"tournament"`
	Teams      []Team         `json:"teams"`
}

type RunSummary struct {
	Slug           string
	Format         string
	DeckCount      int
	OutputJSONPath string
}
