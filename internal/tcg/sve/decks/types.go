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
	Slug         string
	Name         string
	Date         string
	Location     string
	Format       string
	EntryCount   int
	TeamCount    int
	DecksTotal   int
	DecksScraped int
	DecksFailed  int
	SourceURL    string
	CrawledAt    time.Time
}

type TournamentInfo struct {
	Format       string    `json:"format"`
	Location     string    `json:"location,omitempty"`
	EntryCount   int       `json:"entry_count,omitempty"`
	TeamCount    int       `json:"team_count,omitempty"`
	DecksTotal   int       `json:"decks_total"`
	DecksScraped int       `json:"decks_scraped"`
	DecksFailed  int       `json:"decks_failed,omitempty"`
	CrawledAt    time.Time `json:"crawled_at"`
}

type TournamentFile struct {
	Title       string             `json:"title"`
	Link        string             `json:"link"`
	Date        string             `json:"date"`
	Identifier  string             `json:"identifier"`
	Tournament  TournamentInfo     `json:"tournament"`
	Decks       json.RawMessage    `json:"decks"`
	FailedDecks []DeckFetchFailure `json:"failed_decks,omitempty"`
}

type DeckFetchFailure struct {
	DeckID string `json:"deck_id"`
	Reason string `json:"reason"`
}

type SoloEntry struct {
	Ranking                int    `json:"ranking"`
	RankLabel              string `json:"rank_label"`
	TournamentParticipants int    `json:"tournament_participants"`
	Player                 string `json:"player"`
	Craft                  string `json:"craft"`
	Wins                   int    `json:"wins"`
	WinScope               string `json:"win_scope"`
	StandingLabel          string `json:"standing_label"`
	DeckID                 string `json:"deck_id"`
	DecklogURL             string `json:"decklog_url"`
	DeckFetchError         string `json:"deck_fetch_error,omitempty"`
	Deck                   Deck   `json:"deck,omitempty"`
}

type TeamDeck struct {
	Player         string `json:"player,omitempty"`
	Craft          string `json:"craft"`
	DeckID         string `json:"deck_id"`
	DecklogURL     string `json:"decklog_url"`
	DeckFetchError string `json:"deck_fetch_error,omitempty"`
	Deck           Deck   `json:"deck,omitempty"`
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

type RunSummary struct {
	Slug           string
	Format         string
	DecksTotal     int
	DecksScraped   int
	DecksFailed    int
	OutputJSONPath string
}
