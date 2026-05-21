package scraper

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseTextSearchPage(t *testing.T) {
	t.Parallel()

	baseURL := mustParseURL(t, "https://en.shadowverse-evolve.com/cards/searchresults/?expansion_name=BP17&view=text")
	body := []byte(`
<script>var max_page = 15;</script>
<ul class="cardlist-Result_List cardlist-Result_List_Txt">
  <li>
    <a href="/cards/?cardno=BP17-001EN&view=text">
      <div class="img"><img src="/images/BP17-001EN.png" title="Arisa, Evergreen Arrow"></div>
      <div class="txt">
        <p class="number">BP17-001EN</p>
        <p class="ttl">Arisa, Evergreen Arrow</p>
        <div class="status">
          <span class="status-Item">Follower</span>
          <span class="status-Item">Elf</span>
          <span class="status-Item">Legendary</span>
          <span class="status-Item status-Item-Cost"><span class="heading">Cost</span>1</span>
          <span class="status-Item status-Item-Power"><span class="heading">Attack</span>2</span>
          <span class="status-Item status-Item-Hp"><span class="heading">Defense</span>2</span>
        </div>
        <div class="detail"><p><img alt="[fanfare]" class="icon-square" src="/wordpress/wp-content/images/texticon/icon_fanfare.png" />First line.<br />Second line.</p></div>
        <div class="speech">Token text.</div>
      </div>
    </a>
  </li>
</ul>`)

	result, err := parseTextSearchPage(baseURL, body)
	if err != nil {
		t.Fatalf("parseTextSearchPage() error = %v", err)
	}

	if result.maxPage != 15 {
		t.Fatalf("parseTextSearchPage() maxPage = %d, want 15", result.maxPage)
	}
	if len(result.cards) != 1 {
		t.Fatalf("parseTextSearchPage() len = %d, want 1", len(result.cards))
	}

	card := result.cards[0]
	if card.ID != "BP17-001EN" {
		t.Fatalf("card.ID = %q, want BP17-001EN", card.ID)
	}
	if !strings.Contains(card.Effects, "First line.<br/>Second line.") {
		t.Fatalf("card.Effects = %q", card.Effects)
	}
	if len(card.Abilities) == 0 || card.Abilities[0] != "Fanfare" {
		t.Fatalf("card.Abilities = %#v, want Fanfare", card.Abilities)
	}
}

func TestParseTextSearchFragment(t *testing.T) {
	t.Parallel()

	baseURL := mustParseURL(t, "https://en.shadowverse-evolve.com/cards/searchresults/?expansion_name=BP17&view=text")
	body := []byte(`
<li class="ex-item">
  <a href="/cards/?cardno=BP17-016EN&view=text">
    <div class="img"><img src="/images/BP17-016EN.png" title="Elf Sorcerer"></div>
    <div class="txt">
      <p class="number">BP17-016EN</p>
      <p class="ttl">Elf Sorcerer</p>
      <div class="status">
        <span class="status-Item">Follower</span>
        <span class="status-Item">Elf</span>
        <span class="status-Item">Bronze</span>
        <span class="status-Item status-Item-Cost"><span class="heading">Cost</span>6</span>
        <span class="status-Item status-Item-Power"><span class="heading">Attack</span>5</span>
        <span class="status-Item status-Item-Hp"><span class="heading">Defense</span>5</span>
      </div>
      <div class="detail"><p>Deal damage.</p></div>
    </div>
  </a>
</li>`)

	cards, err := parseTextSearchFragment(baseURL, body)
	if err != nil {
		t.Fatalf("parseTextSearchFragment() error = %v", err)
	}

	if len(cards) != 1 {
		t.Fatalf("parseTextSearchFragment() len = %d, want 1", len(cards))
	}
	if cards[0].Name != "Elf Sorcerer" {
		t.Fatalf("cards[0].Name = %q, want Elf Sorcerer", cards[0].Name)
	}
	if cards[0].Cost == nil || *cards[0].Cost != 6 {
		t.Fatalf("cards[0].Cost = %#v, want 6", cards[0].Cost)
	}
}

func TestBuildTextViewURL(t *testing.T) {
	t.Parallel()

	searchURL := mustParseURL(t, "https://en.shadowverse-evolve.com/cards/searchresults/?expansion_name=BP17")
	got := buildTextViewURL(searchURL).String()

	if !strings.Contains(got, "view=text") {
		t.Fatalf("buildTextViewURL() = %q", got)
	}
	if !strings.Contains(got, "sort=new") {
		t.Fatalf("buildTextViewURL() = %q", got)
	}
}

func TestBuildSearchResultsExURL(t *testing.T) {
	t.Parallel()

	searchURL := mustParseURL(t, "https://en.shadowverse-evolve.com/cards/searchresults/?expansion_name=BP17&view=text")
	got, err := buildSearchResultsExURL(searchURL, 2)
	if err != nil {
		t.Fatalf("buildSearchResultsExURL() error = %v", err)
	}

	if !strings.Contains(got, "/cards/searchresults_ex?") {
		t.Fatalf("buildSearchResultsExURL() = %q", got)
	}
	if !strings.Contains(got, "page=2") || !strings.Contains(got, "view=text") {
		t.Fatalf("buildSearchResultsExURL() = %q", got)
	}
}

func TestBuildImageName(t *testing.T) {
	t.Parallel()

	got := buildImageName("BP17-007EN")
	want := "BP17-007EN.webp"
	if got != want {
		t.Fatalf("buildImageName() = %q, want %q", got, want)
	}
}

func TestRun(t *testing.T) {
	imageBytes := pngBytes(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/cards/searchresults/":
			_, _ = w.Write([]byte(`
<script>var max_page = 2;</script>
<ul class="cardlist-Result_List cardlist-Result_List_Txt">
  <li>
    <a href="/cards/?cardno=BP17-001EN&view=text">
      <div class="img"><img src="/images/BP17-001EN.png" title="Arisa, Evergreen Arrow"></div>
      <div class="txt">
        <p class="number">BP17-001EN</p>
        <p class="ttl">Arisa, Evergreen Arrow</p>
        <div class="status">
          <span class="status-Item">Follower</span>
          <span class="status-Item">Elf</span>
          <span class="status-Item">Legendary</span>
          <span class="status-Item status-Item-Cost"><span class="heading">Cost</span>1</span>
          <span class="status-Item status-Item-Power"><span class="heading">Attack</span>2</span>
          <span class="status-Item status-Item-Hp"><span class="heading">Defense</span>2</span>
        </div>
        <div class="detail"><p>Arisa, Evergreen Arrow effect.</p></div>
      </div>
    </a>
  </li>
  <li>
    <a href="/cards/?cardno=BP17-002EN&view=text">
      <div class="img"><img src="/images/BP17-002EN.png" title="Ladica, Verdant Claw"></div>
      <div class="txt">
        <p class="number">BP17-002EN</p>
        <p class="ttl">Ladica, Verdant Claw</p>
        <div class="status">
          <span class="status-Item">Follower</span>
          <span class="status-Item">Natura / Beast</span>
          <span class="status-Item">Legendary</span>
          <span class="status-Item status-Item-Cost"><span class="heading">Cost</span>3</span>
          <span class="status-Item status-Item-Power"><span class="heading">Attack</span>3</span>
          <span class="status-Item status-Item-Hp"><span class="heading">Defense</span>3</span>
        </div>
        <div class="detail"><p>Ladica, Verdant Claw effect.</p></div>
      </div>
    </a>
  </li>
</ul>`))
		case r.URL.Path == "/cards/searchresults_ex":
			_, _ = w.Write([]byte(`
<li class="ex-item">
  <a href="/cards/?cardno=BP17-003EN&view=text">
    <div class="img"><img src="/images/BP17-003EN.png" title="Setus, Sunlit Hero"></div>
    <div class="txt">
      <p class="number">BP17-003EN</p>
      <p class="ttl">Setus, Sunlit Hero</p>
      <div class="status">
        <span class="status-Item">Follower</span>
        <span class="status-Item">Beast</span>
        <span class="status-Item">Legendary</span>
        <span class="status-Item status-Item-Cost"><span class="heading">Cost</span>4</span>
        <span class="status-Item status-Item-Power"><span class="heading">Attack</span>4</span>
        <span class="status-Item status-Item-Hp"><span class="heading">Defense</span>5</span>
      </div>
      <div class="detail"><p>Setus, Sunlit Hero effect.</p></div>
      <div class="speech">Token text.</div>
    </div>
  </a>
</li>`))
		case strings.HasPrefix(r.URL.Path, "/images/"):
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(imageBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tempDir := t.TempDir()
	cfg := Config{
		SearchURL:        server.URL + "/cards/searchresults/?expansion_name=BP17",
		OutputJSONPath:   filepath.Join(tempDir, "output", "cards.json"),
		ImagesDir:        filepath.Join(tempDir, "output", "images"),
		UserAgent:        "scraper-cli-test",
		WebPQuality:      100,
		ImageConcurrency: 4,
	}

	if err := Run(context.Background(), cfg); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	data, err := os.ReadFile(cfg.OutputJSONPath)
	if err != nil {
		t.Fatalf("ReadFile(cards.json) error = %v", err)
	}

	var output []Card
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if len(output) != 3 {
		t.Fatalf("len(output) = %d, want 3", len(output))
	}
	if strings.Contains(string(data), "\\u003c") {
		t.Fatalf("cards.json escaped HTML tags: %s", string(data))
	}
	if !strings.Contains(output[0].Effects, "Arisa, Evergreen Arrow effect.") {
		t.Fatalf("first effects = %q", output[0].Effects)
	}
	if !strings.Contains(output[2].Effects, "Setus, Sunlit Hero effect.") {
		t.Fatalf("third effects = %q", output[2].Effects)
	}
	if output[2].MainType != "Follower" {
		t.Fatalf("third main_type = %q", output[2].MainType)
	}
	entries, err := os.ReadDir(cfg.ImagesDir)
	if err != nil {
		t.Fatalf("os.ReadDir(%q) error = %v", cfg.ImagesDir, err)
	}
	if len(entries) != 3 {
		t.Fatalf("len(images) = %d, want 3", len(entries))
	}
}

func TestListCards(t *testing.T) {
	t.Parallel()

	imageBytes := pngBytes(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/cards/searchresults/":
			_, _ = w.Write([]byte(`
<script>var max_page = 1;</script>
<ul class="cardlist-Result_List cardlist-Result_List_Txt">
  <li>
    <a href="/cards/?cardno=BP17-001EN&view=text">
      <div class="img"><img src="/images/BP17-001EN.png" title="Arisa, Evergreen Arrow"></div>
      <div class="txt">
        <p class="number">BP17-001EN</p>
        <p class="ttl">Arisa, Evergreen Arrow</p>
        <div class="status">
          <span class="status-Item">Follower</span>
          <span class="status-Item">Elf</span>
          <span class="status-Item">Legendary</span>
          <span class="status-Item status-Item-Cost"><span class="heading">Cost</span>1</span>
          <span class="status-Item status-Item-Power"><span class="heading">Attack</span>2</span>
          <span class="status-Item status-Item-Hp"><span class="heading">Defense</span>2</span>
        </div>
        <div class="detail"><p>Arisa, Evergreen Arrow effect.</p></div>
      </div>
    </a>
  </li>
</ul>`))
		case strings.HasPrefix(r.URL.Path, "/images/"):
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(imageBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := Config{
		SearchURL: server.URL + "/cards/searchresults/?expansion_name=BP17",
		UserAgent: "scraper-cli-test",
	}

	cards, err := ListCards(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ListCards() error = %v", err)
	}

	if len(cards) != 1 {
		t.Fatalf("len(cards) = %d, want 1", len(cards))
	}
	if cards[0].ID != "BP17-001EN" || cards[0].Name != "Arisa, Evergreen Arrow" {
		t.Fatalf("ListCards()[0] = %#v", cards[0])
	}
	if cards[0].ImageFile != "" {
		t.Fatalf("ListCards()[0].ImageFile = %q, want empty", cards[0].ImageFile)
	}
}

func TestDownloadImagesOnly(t *testing.T) {
	t.Parallel()

	imageBytes := pngBytes(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/wordpress/wp-content/images/cardlist/BP17/BP17-001EN.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(imageBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tempDir := t.TempDir()
	imagesDir := filepath.Join(tempDir, "output", "images")
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	stalePath := filepath.Join(imagesDir, "stale.webp")
	if err := os.WriteFile(stalePath, []byte("stale"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(stale) error = %v", err)
	}

	jsonPath := filepath.Join(tempDir, "output", "cards.json")
	cardsJSON := []Card{{
		ID:       "BP17-001EN",
		Name:     "Arisa, Evergreen Arrow",
		SetID:    "BP17",
		Language: "en",
	}}
	data, err := json.Marshal(cardsJSON)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(jsonPath), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(json dir) error = %v", err)
	}
	if err := os.WriteFile(jsonPath, data, 0o644); err != nil {
		t.Fatalf("os.WriteFile(cards.json) error = %v", err)
	}

	cfg := Config{
		SearchURL:        server.URL + "/cards/searchresults/?expansion_name=BP17",
		OutputJSONPath:   jsonPath,
		ImagesDir:        imagesDir,
		UserAgent:        "tcg-scout-test",
		WebPQuality:      80,
		ImageConcurrency: 2,
		PageConcurrency:  1,
	}

	if err := DownloadImagesOnly(context.Background(), cfg); err != nil {
		t.Fatalf("DownloadImagesOnly() error = %v", err)
	}

	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("stale image still exists or wrong error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(imagesDir, "BP17-001EN.webp")); err != nil {
		t.Fatalf("os.Stat(downloaded image) error = %v", err)
	}
}

func TestDownloadImagesOnlyCancelsOutstandingRequestsOnFirstError(t *testing.T) {
	t.Parallel()

	var slowRequestStarted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/wordpress/wp-content/images/cardlist/BP17/BP17-001EN.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("not-an-image"))
		case "/wordpress/wp-content/images/cardlist/BP17/BP17-002EN.png":
			slowRequestStarted.Store(true)
			select {
			case <-r.Context().Done():
				return
			case <-time.After(2 * time.Second):
				t.Error("slow image request was not cancelled after first error")
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tempDir := t.TempDir()
	imagesDir := filepath.Join(tempDir, "output", "images")
	jsonPath := filepath.Join(tempDir, "output", "cards.json")
	cardsJSON := []Card{
		{ID: "BP17-001EN", Name: "Bad Image", SetID: "BP17", Language: "en"},
		{ID: "BP17-002EN", Name: "Slow Image", SetID: "BP17", Language: "en"},
	}
	data, err := json.Marshal(cardsJSON)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(jsonPath), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(json dir) error = %v", err)
	}
	if err := os.WriteFile(jsonPath, data, 0o644); err != nil {
		t.Fatalf("os.WriteFile(cards.json) error = %v", err)
	}

	cfg := Config{
		SearchURL:        server.URL + "/cards/searchresults/?expansion_name=BP17",
		OutputJSONPath:   jsonPath,
		ImagesDir:        imagesDir,
		UserAgent:        "tcg-scout-test",
		WebPQuality:      80,
		ImageConcurrency: 2,
		PageConcurrency:  1,
	}

	start := time.Now()
	err = DownloadImagesOnly(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "decode image for BP17-001EN") {
		t.Fatalf("DownloadImagesOnly() error = %v", err)
	}
	if !slowRequestStarted.Load() {
		t.Fatalf("slow image request did not start")
	}
	if elapsed := time.Since(start); elapsed >= 2*time.Second {
		t.Fatalf("DownloadImagesOnly() took %s, expected cancellation before 2s", elapsed)
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	return parsed
}

func pngBytes(t *testing.T) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 0, color.RGBA{G: 255, A: 255})
	img.Set(0, 1, color.RGBA{B: 255, A: 255})
	img.Set(1, 1, color.RGBA{R: 255, G: 255, B: 255, A: 255})

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode() error = %v", err)
	}

	return buf.Bytes()
}
