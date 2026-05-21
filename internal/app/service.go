package app

import (
	"context"
	"fmt"
	"sort"

	"tcg-scout/internal/scraper"
)

const (
	ActionScrape = "scrape"
	ActionList   = "list"
	ActionImages = "images"
)

type Request struct {
	SearchURL        string  `json:"search_url,omitempty"`
	OutputJSONPath   string  `json:"output_json_path,omitempty"`
	ImagesDir        string  `json:"images_dir,omitempty"`
	UserAgent        string  `json:"user_agent,omitempty"`
	AllowedDomain    string  `json:"allowed_domain,omitempty"`
	WebPQuality      float64 `json:"webp_quality,omitempty"`
	ImageConcurrency int     `json:"image_concurrency,omitempty"`
	PageConcurrency  int     `json:"page_concurrency,omitempty"`
}

func DefaultRequest() Request {
	return Request{
		SearchURL:        scraper.DefaultSearchURL,
		OutputJSONPath:   "output/cards.json",
		ImagesDir:        "output/images",
		UserAgent:        "tcg-scout/1.0",
		WebPQuality:      100,
		ImageConcurrency: 8,
		PageConcurrency:  4,
	}
}

func (r Request) ScraperConfig() scraper.Config {
	return scraper.Config{
		SearchURL:        r.SearchURL,
		OutputJSONPath:   r.OutputJSONPath,
		ImagesDir:        r.ImagesDir,
		UserAgent:        r.UserAgent,
		AllowedDomain:    r.AllowedDomain,
		WebPQuality:      r.WebPQuality,
		ImageConcurrency: r.ImageConcurrency,
		PageConcurrency:  r.PageConcurrency,
	}
}

type Result struct {
	Summary Summary        `json:"summary"`
	Cards   []scraper.Card `json:"cards,omitempty"`
}

type Summary struct {
	Game           string `json:"game"`
	Resource       string `json:"resource"`
	Scraper        string `json:"scraper"`
	Action         string `json:"action"`
	CardCount      int    `json:"card_count,omitempty"`
	OutputJSONPath string `json:"output_json_path,omitempty"`
	ImagesDir      string `json:"images_dir,omitempty"`
	Message        string `json:"message"`
}

type ActionDefinition struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
}

type ScraperDefinition struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	Summary       string             `json:"summary"`
	DefaultAction string             `json:"default_action"`
	Actions       []ActionDefinition `json:"actions"`
}

type ResourceDefinition struct {
	ID       string              `json:"id"`
	Name     string              `json:"name"`
	Summary  string              `json:"summary"`
	Scrapers []ScraperDefinition `json:"scrapers,omitempty"`
}

type GameDefinition struct {
	ID        string               `json:"id"`
	Name      string               `json:"name"`
	Summary   string               `json:"summary"`
	Resources []ResourceDefinition `json:"resources,omitempty"`
}

type RunnerDefinition struct {
	GameID          string
	GameName        string
	GameSummary     string
	ResourceID      string
	ResourceName    string
	ResourceSummary string
	ScraperID       string
	ScraperName     string
	ScraperSummary  string
	DefaultAction   string
	Actions         []ActionDefinition
}

type Runner interface {
	Definition() RunnerDefinition
	Execute(context.Context, string, Request) (Result, error)
}

type Selection struct {
	Game     string `json:"game"`
	Resource string `json:"resource"`
	Scraper  string `json:"scraper"`
}

type Service struct {
	games map[string]*gameNode
}

type gameNode struct {
	meta      GameDefinition
	resources map[string]*resourceNode
}

type resourceNode struct {
	meta     ResourceDefinition
	scrapers map[string]*scraperNode
}

type scraperNode struct {
	meta   ScraperDefinition
	runner Runner
}

func NewService(runners ...Runner) (*Service, error) {
	svc := &Service{
		games: make(map[string]*gameNode),
	}

	for _, runner := range runners {
		def := runner.Definition()
		game := svc.games[def.GameID]
		if game == nil {
			game = &gameNode{
				meta: GameDefinition{
					ID:      def.GameID,
					Name:    def.GameName,
					Summary: def.GameSummary,
				},
				resources: make(map[string]*resourceNode),
			}
			svc.games[def.GameID] = game
		}

		resource := game.resources[def.ResourceID]
		if resource == nil {
			resource = &resourceNode{
				meta: ResourceDefinition{
					ID:      def.ResourceID,
					Name:    def.ResourceName,
					Summary: def.ResourceSummary,
				},
				scrapers: make(map[string]*scraperNode),
			}
			game.resources[def.ResourceID] = resource
		}

		if _, exists := resource.scrapers[def.ScraperID]; exists {
			return nil, fmt.Errorf("duplicate scraper %s/%s/%s", def.GameID, def.ResourceID, def.ScraperID)
		}

		resource.scrapers[def.ScraperID] = &scraperNode{
			meta: ScraperDefinition{
				ID:            def.ScraperID,
				Name:          def.ScraperName,
				Summary:       def.ScraperSummary,
				DefaultAction: def.DefaultAction,
				Actions:       append([]ActionDefinition(nil), def.Actions...),
			},
			runner: runner,
		}
	}

	return svc, nil
}

func (s *Service) Games() []GameDefinition {
	gameIDs := make([]string, 0, len(s.games))
	for id := range s.games {
		gameIDs = append(gameIDs, id)
	}
	sort.Strings(gameIDs)

	games := make([]GameDefinition, 0, len(gameIDs))
	for _, gameID := range gameIDs {
		game := s.games[gameID]
		gameDef := game.meta
		gameDef.Resources = s.Resources(gameID)
		games = append(games, gameDef)
	}

	return games
}

func (s *Service) Resources(gameID string) []ResourceDefinition {
	game := s.games[gameID]
	if game == nil {
		return nil
	}

	resourceIDs := make([]string, 0, len(game.resources))
	for id := range game.resources {
		resourceIDs = append(resourceIDs, id)
	}
	sort.Strings(resourceIDs)

	resources := make([]ResourceDefinition, 0, len(resourceIDs))
	for _, resourceID := range resourceIDs {
		resource := game.resources[resourceID]
		resourceDef := resource.meta
		resourceDef.Scrapers = s.Scrapers(gameID, resourceID)
		resources = append(resources, resourceDef)
	}

	return resources
}

func (s *Service) Scrapers(gameID, resourceID string) []ScraperDefinition {
	game := s.games[gameID]
	if game == nil {
		return nil
	}
	resource := game.resources[resourceID]
	if resource == nil {
		return nil
	}

	scraperIDs := make([]string, 0, len(resource.scrapers))
	for id := range resource.scrapers {
		scraperIDs = append(scraperIDs, id)
	}
	sort.Strings(scraperIDs)

	scrapers := make([]ScraperDefinition, 0, len(scraperIDs))
	for _, scraperID := range scraperIDs {
		scrapers = append(scrapers, resource.scrapers[scraperID].meta)
	}

	return scrapers
}

func (s *Service) Execute(ctx context.Context, selection Selection, action string, req Request) (Result, error) {
	game := s.games[selection.Game]
	if game == nil {
		return Result{}, fmt.Errorf("unknown game %q", selection.Game)
	}

	resource := game.resources[selection.Resource]
	if resource == nil {
		return Result{}, fmt.Errorf("unknown resource %q for game %q", selection.Resource, selection.Game)
	}

	scraperNode := resource.scrapers[selection.Scraper]
	if scraperNode == nil {
		return Result{}, fmt.Errorf("unknown scraper %q for %s %s", selection.Scraper, selection.Game, selection.Resource)
	}

	if !supportsAction(scraperNode.meta, action) {
		return Result{}, fmt.Errorf("unknown action %q for %s %s %s", action, selection.Game, selection.Resource, selection.Scraper)
	}

	result, err := scraperNode.runner.Execute(ctx, action, req)
	if err != nil {
		return Result{}, err
	}
	if result.Summary.Game == "" {
		result.Summary.Game = selection.Game
	}
	if result.Summary.Resource == "" {
		result.Summary.Resource = selection.Resource
	}
	if result.Summary.Scraper == "" {
		result.Summary.Scraper = selection.Scraper
	}
	if result.Summary.Action == "" {
		result.Summary.Action = action
	}

	return result, nil
}

func supportsAction(def ScraperDefinition, action string) bool {
	for _, candidate := range def.Actions {
		if candidate.ID == action {
			return true
		}
	}
	return false
}
