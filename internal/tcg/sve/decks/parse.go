package decks

import (
	"bytes"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

var rankSuffixPattern = regexp.MustCompile(`(\d+)`)

type ParsedPage struct {
	Meta   TournamentMeta
	Format string
	Solo   []SoloEntry
	Teams  []Team
}

func ParseTournamentPage(pageURL string, html []byte) (ParsedPage, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(html))
	if err != nil {
		return ParsedPage{}, fmt.Errorf("parse html: %w", err)
	}

	meta, err := parseTournamentMeta(pageURL, doc)
	if err != nil {
		return ParsedPage{}, err
	}

	if doc.Find("#team-table").Length() > 0 {
		teams, err := parseTeamTable(doc)
		if err != nil {
			return ParsedPage{}, err
		}
		meta.Format = FormatTeam
		meta.TeamCount = len(teams)
		return ParsedPage{Meta: meta, Format: FormatTeam, Teams: teams}, nil
	}

	entries, err := parseSoloTable(doc)
	if err != nil {
		return ParsedPage{}, err
	}
	meta.Format = FormatSolo
	meta.EntryCount = len(entries)
	return ParsedPage{Meta: meta, Format: FormatSolo, Solo: entries}, nil
}

func parseTournamentMeta(pageURL string, doc *goquery.Document) (TournamentMeta, error) {
	parsed, err := url.Parse(pageURL)
	if err != nil {
		return TournamentMeta{}, fmt.Errorf("parse tournament url: %w", err)
	}

	slug := strings.Trim(strings.TrimSuffix(parsed.Path, "/"), "/")
	if idx := strings.LastIndex(slug, "/"); idx >= 0 {
		slug = slug[idx+1:]
	}
	if slug == "" {
		return TournamentMeta{}, fmt.Errorf("could not derive tournament slug from %q", pageURL)
	}

	title := strings.TrimSpace(doc.Find(".detail-Ttl").First().Text())
	if title == "" {
		title = strings.TrimSpace(doc.Find("title").First().Text())
		title = strings.TrimSuffix(title, " | DECKS | Shadowverse: Evolve")
	}

	date := strings.TrimSpace(doc.Find(".date-Category .date").First().Text())

	link := pageURL
	if canonical, ok := doc.Find(`link[rel="canonical"]`).Attr("href"); ok {
		if trimmed := strings.TrimSpace(canonical); trimmed != "" {
			link = trimmed
		}
	}

	location := ""
	doc.Find("h1").EachWithBreak(func(_ int, sel *goquery.Selection) bool {
		if sel.HasClass("section-header") {
			return true
		}
		text := strings.TrimSpace(sel.Text())
		if text == "" || text == "GALLERY" || text == "DECKS" {
			return true
		}
		location = text
		return false
	})

	return TournamentMeta{
		Slug:      slug,
		Name:      title,
		Date:      date,
		Location:  location,
		SourceURL: link,
	}, nil
}

func parseSoloTable(doc *goquery.Document) ([]SoloEntry, error) {
	var entries []SoloEntry

	doc.Find("tr.decklog-frame-trigger").Each(func(_ int, row *goquery.Selection) {
		deckID := strings.TrimSpace(row.AttrOr("data-deck-id", ""))
		if deckID == "" {
			return
		}

		rankLabel := strings.TrimSpace(row.Find(".rank-cell").First().Text())
		wins := parseInt(row.Find(".win-ratio").First().Text())
		entries = append(entries, SoloEntry{
			Ranking:       parseRankOrdinal(rankLabel),
			RankLabel:     rankLabel,
			Player:        strings.TrimSpace(row.Find(".player-name").First().Text()),
			Craft:         strings.TrimSpace(row.Find(".nation-name").First().Text()),
			Wins:          wins,
			WinScope:      WinScopePlayer,
			StandingLabel: soloStandingLabel(rankLabel, wins),
			DeckID:        deckID,
			DecklogURL:    DecklogViewBase + deckID,
		})
	})

	if len(entries) == 0 {
		return nil, fmt.Errorf("no solo deck entries found")
	}
	return entries, nil
}

func parseTeamTable(doc *goquery.Document) ([]Team, error) {
	var teams []Team

	doc.Find("#team-table tbody tr").Each(func(_ int, row *goquery.Selection) {
		if row.Find(".team-banner-cell").Length() > 0 {
			team := Team{
				RankLabel: strings.TrimSpace(row.Find(".rank-cell").First().Text()),
				TeamName:  strings.TrimSpace(row.Find(".team-banner-cell span").First().Text()),
				TeamWins:  parseInt(row.Find(".win-ratio").First().Text()),
			}
			team.Rank = parseRankOrdinal(team.RankLabel)
			finalizeTeam(&team)
			teams = append(teams, team)
			return
		}

		deckCell := row.Find("td.decklog-frame-trigger")
		if deckCell.Length() == 0 {
			return
		}

		deckID := strings.TrimSpace(deckCell.AttrOr("data-deck-id", ""))
		if deckID == "" {
			return
		}

		deck := TeamDeck{
			DeckID:     deckID,
			Craft:      strings.TrimSpace(deckCell.Find(".nation-name").Text()),
			DecklogURL: DecklogViewBase + deckID,
		}

		rankCell := row.Find(".rank-cell")
		if rankCell.Length() > 0 {
			team := Team{
				RankLabel: strings.TrimSpace(rankCell.Text()),
				TeamWins:  parseInt(row.Find(".win-ratio").First().Text()),
			}
			team.Rank = parseRankOrdinal(team.RankLabel)

			playerCell := row.Find(".player-name").First()
			if _, hasRowspan := playerCell.Attr("rowspan"); hasRowspan {
				team.TeamName = strings.TrimSpace(playerCell.Text())
			} else {
				deck.Player = strings.TrimSpace(playerCell.Text())
			}

			finalizeTeam(&team)
			team.Decks = append(team.Decks, deck)
			teams = append(teams, team)
			return
		}

		if len(teams) == 0 {
			return
		}

		playerCell := row.Find(".player-name").First()
		if _, hasRowspan := playerCell.Attr("rowspan"); !hasRowspan {
			deck.Player = strings.TrimSpace(playerCell.Text())
		}

		last := len(teams) - 1
		teams[last].Decks = append(teams[last].Decks, deck)
	})

	if len(teams) == 0 {
		return nil, fmt.Errorf("no team entries found")
	}
	return teams, nil
}

func finalizeTeam(team *Team) {
	team.WinScope = WinScopeTeam
	team.StandingLabel = teamStandingLabel(team.RankLabel, team.TeamWins)
}

func soloStandingLabel(rankLabel string, wins int) string {
	return fmt.Sprintf("%s place · %d wins", strings.TrimSuffix(rankLabel, " "), wins)
}

func teamStandingLabel(rankLabel string, teamWins int) string {
	return fmt.Sprintf("%s place team · %d team wins", strings.TrimSuffix(rankLabel, " "), teamWins)
}

func parseRankOrdinal(label string) int {
	match := rankSuffixPattern.FindStringSubmatch(label)
	if len(match) < 2 {
		return 0
	}
	value, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	return value
}

func parseInt(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func collectDeckIDs(page ParsedPage) []string {
	seen := make(map[string]struct{})
	var ids []string

	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, exists := seen[id]; exists {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	switch page.Format {
	case FormatSolo:
		for _, entry := range page.Solo {
			add(entry.DeckID)
		}
	case FormatTeam:
		for _, team := range page.Teams {
			for _, deck := range team.Decks {
				add(deck.DeckID)
			}
		}
	}

	return ids
}
