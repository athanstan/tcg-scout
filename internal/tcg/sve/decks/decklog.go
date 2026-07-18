package decks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

var defaultHTTPClient = &http.Client{Timeout: 30 * time.Second}

type DecklogClient struct {
	httpClient *http.Client
	userAgent  string
	apiBaseURL string
}

func NewDecklogClient(cfg Config) *DecklogClient {
	client := cfg.HTTPClient
	if client == nil {
		client = defaultHTTPClient
	}

	apiBase := cfg.DecklogAPIBase
	if apiBase == "" {
		apiBase = defaultDecklogAPIBase
	}

	userAgent := cfg.UserAgent
	if userAgent == "" {
		userAgent = DefaultUserAgent
	}

	return &DecklogClient{
		httpClient: client,
		userAgent:  userAgent,
		apiBaseURL: apiBase,
	}
}

func (c *DecklogClient) FetchDeck(ctx context.Context, deckID string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBaseURL+deckID, nil)
	if err != nil {
		return nil, fmt.Errorf("build decklog request for %s: %w", deckID, err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", DecklogReferer)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch deck %s: %w", deckID, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read deck %s response: %w", deckID, err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("deck %s: unexpected status %s", deckID, resp.Status)
	}

	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" || trimmed == "[]" {
		return nil, fmt.Errorf("deck %s: empty decklog response", deckID)
	}

	if !json.Valid([]byte(trimmed)) {
		return nil, fmt.Errorf("deck %s: invalid json response", deckID)
	}

	return json.RawMessage(trimmed), nil
}

type DeckFetchBatch struct {
	Decks    map[string]json.RawMessage
	Failures []DeckFetchFailure
}

func (c *DecklogClient) FetchDecks(ctx context.Context, deckIDs []string, concurrency int) (DeckFetchBatch, error) {
	if concurrency <= 0 {
		concurrency = DefaultDeckConcurrency
	}

	batch := DeckFetchBatch{
		Decks: make(map[string]json.RawMessage, len(deckIDs)),
	}
	var mu sync.Mutex

	group, ctx := errgroup.WithContext(ctx)
	group.SetLimit(concurrency)

	for _, deckID := range deckIDs {
		deckID := deckID
		group.Go(func() error {
			if err := ctx.Err(); err != nil {
				return err
			}

			deck, err := c.FetchDeck(ctx, deckID)
			if err != nil {
				mu.Lock()
				batch.Failures = append(batch.Failures, DeckFetchFailure{
					DeckID: deckID,
					Reason: deckFetchReason(deckID, err),
				})
				mu.Unlock()
				return nil
			}

			mu.Lock()
			batch.Decks[deckID] = deck
			mu.Unlock()
			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return DeckFetchBatch{}, err
	}
	return batch, nil
}

func deckFetchReason(deckID string, err error) string {
	prefix := "deck " + deckID + ": "
	return strings.TrimPrefix(err.Error(), prefix)
}

func httpClientFor(cfg Config) *http.Client {
	if cfg.HTTPClient != nil {
		return cfg.HTTPClient
	}
	return defaultHTTPClient
}
