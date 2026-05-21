package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tcg-scout/internal/app"
	"tcg-scout/internal/scraper"
)

type fakeRunner struct {
	err error
}

func (f fakeRunner) Definition() app.RunnerDefinition {
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
		},
	}
}

func (f fakeRunner) Execute(_ context.Context, _ string, _ app.Request) (app.Result, error) {
	if f.err != nil {
		return app.Result{}, f.err
	}
	return app.Result{
		Summary: app.Summary{Game: "demo", Resource: "cards", Scraper: "alpha", Action: app.ActionList, CardCount: 1, Message: "listed 1 cards"},
		Cards:   []scraper.Card{{ID: "C-001", Name: "Alpha Card"}},
	}, nil
}

func TestServerHandler(t *testing.T) {
	tests := []struct {
		name         string
		runner       fakeRunner
		method       string
		target       string
		body         string
		wantStatus   int
		wantContains []string
	}{
		{
			name:         "health",
			method:       http.MethodGet,
			target:       "/healthz",
			wantStatus:   http.StatusOK,
			wantContains: []string{`"status": "ok"`},
		},
		{
			name:         "list games",
			method:       http.MethodGet,
			target:       "/v1/games",
			wantStatus:   http.StatusOK,
			wantContains: []string{`"demo"`},
		},
		{
			name:         "resources not found",
			method:       http.MethodGet,
			target:       "/v1/games/missing/resources",
			wantStatus:   http.StatusNotFound,
			wantContains: []string{`unknown game \"missing\"`},
		},
		{
			name:         "scrapers listed",
			method:       http.MethodGet,
			target:       "/v1/games/demo/resources/cards/scrapers",
			wantStatus:   http.StatusOK,
			wantContains: []string{`"alpha"`},
		},
		{
			name:         "run success",
			method:       http.MethodPost,
			target:       "/v1/runs",
			body:         `{"game":"demo","resource":"cards","scraper":"alpha","action":"list"}`,
			wantStatus:   http.StatusOK,
			wantContains: []string{`"Alpha Card"`},
		},
		{
			name:         "run invalid json",
			method:       http.MethodPost,
			target:       "/v1/runs",
			body:         `{`,
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`decode request`},
		},
		{
			name:         "run invalid selection",
			method:       http.MethodPost,
			target:       "/v1/runs",
			body:         `{"game":"demo","resource":"cards","scraper":"alpha","action":"images"}`,
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`unknown action \"images\"`},
		},
		{
			name:         "run execution failure",
			runner:       fakeRunner{err: errors.New("runner failed")},
			method:       http.MethodPost,
			target:       "/v1/runs",
			body:         `{"game":"demo","resource":"cards","scraper":"alpha","action":"list"}`,
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`runner failed`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := newTestServer(t, tt.runner)
			request := httptest.NewRequest(tt.method, tt.target, bytes.NewBufferString(tt.body))
			recorder := httptest.NewRecorder()

			server.Handler().ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d body = %s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(recorder.Body.String(), want) {
					t.Fatalf("body = %s, missing %q", recorder.Body.String(), want)
				}
			}
		})
	}
}

func TestNewServer(t *testing.T) {
	t.Parallel()

	service, err := app.NewService(fakeRunner{})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = NewServer(service, WithAddress(""))
	if err == nil || !strings.Contains(err.Error(), "server address is required") {
		t.Fatalf("NewServer() error = %v", err)
	}
}

func newTestServer(t *testing.T, runner fakeRunner) *Server {
	t.Helper()

	service, err := app.NewService(runner)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	server, err := NewServer(service)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	return server
}
