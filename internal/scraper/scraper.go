package scraper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chai2010/webp"
	"github.com/gocolly/colly/v2"
	"golang.org/x/net/html"
	"golang.org/x/sync/errgroup"
)

const (
	DefaultSearchURL = "https://en.shadowverse-evolve.com/cards/searchresults/?card_name=&format%5B%5D=all&class%5B%5D=all&title=&expansion_name=BP17&cost%5B%5D=all&card_kind%5B%5D=all&rare%5B%5D=all"
	defaultSetOrder  = 37
	defaultLanguage  = "en"
)

var (
	maxPagePattern        = regexp.MustCompile(`var\s+max_page\s*=\s*(\d+)`)
	nonSlugPattern        = regexp.MustCompile(`[^a-z0-9]+`)
	setNamePattern        = regexp.MustCompile(`Cards with the card set "(.*)"`)
	effectIconPathPattern = regexp.MustCompile(`/[^"]*?/icon_([a-z0-9]+)\.png`)
	abilityTextPattern    = regexp.MustCompile(`\b(On Evolve|Last Words|Fanfare|Necrocharge|Storm|Ward|Assail|Rush|Intimidate|Drain|Bane|Ambush|Strike|Clash|Enhance)\b`)
	effectIconAltPattern  = regexp.MustCompile(`alt="\[([a-z0-9]+)\]"`)
	craftNames            = []string{"Forestcraft", "Swordcraft", "Runecraft", "Dragoncraft", "Abysscraft", "Havencraft", "Neutral"}
	craftIDs              = map[string]int{"Forestcraft": 1, "Swordcraft": 2, "Runecraft": 3, "Dragoncraft": 4, "Abysscraft": 5, "Havencraft": 6, "Neutral": 7}
	rarityIDs             = map[string]int{"-": 100, "Bronze": 1, "Bronze / Premium": 2, "Silver": 3, "Silver / Premium": 4, "Gold": 5, "Gold / Premium": 6, "Legendary": 7, "Super Legendary": 8, "Ultimate": 9, "Special": 10, "Premium": 13}
	effectIconAbilities   = map[string]string{"fanfare": "Fanfare", "lastwords": "Last Words", "strike": "Strike", "clash": "Clash", "enhance": "Enhance"}
	httpClient            = &http.Client{Timeout: 30 * time.Second}
)

type Config struct {
	SearchURL        string
	OutputJSONPath   string
	ImagesDir        string
	UserAgent        string
	WebPQuality      float64
	ImageConcurrency int
	PageConcurrency  int
	AllowedDomain    string
	NamesOnly        bool
	ImagesOnly       bool
}

type Card struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Rarity         string   `json:"rarity"`
	RarityID       int      `json:"rarity_id"`
	Cost           *int     `json:"cost"`
	MainType       string   `json:"main_type"`
	SubType        *string  `json:"sub_type"`
	SetID          string   `json:"set_id"`
	SetName        string   `json:"set_name"`
	SetOrder       int      `json:"set_order"`
	CraftName      string   `json:"craft_name"`
	CraftID        int      `json:"craft_id"`
	Language       string   `json:"language"`
	Abilities      []string `json:"abilities"`
	Effects        string   `json:"effects"`
	Traits         []string `json:"traits"`
	Atk            *int     `json:"atk"`
	Health         *int     `json:"health"`
	DetailURL      string   `json:"-"`
	SourceImageURL string   `json:"-"`
	ImageFile      string   `json:"-"`
}

type pageBundle struct {
	cards   []Card
	maxPage int
	setName string
}

func Run(ctx context.Context, cfg Config) error {
	_, err := Scrape(ctx, cfg)
	return err
}

type RunSummary struct {
	CardCount      int
	OutputJSONPath string
	ImagesDir      string
}

func Scrape(ctx context.Context, cfg Config) (RunSummary, error) {
	cfg, cards, cancel, err := prepareCards(ctx, cfg)
	if err != nil {
		return RunSummary{}, err
	}
	defer cancel()

	statusf("Preparing %d cards for image download", len(cards))

	if err := os.MkdirAll(cfg.ImagesDir, 0o755); err != nil {
		return RunSummary{}, fmt.Errorf("create images directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.OutputJSONPath), 0o755); err != nil {
		return RunSummary{}, fmt.Errorf("create output directory: %w", err)
	}

	var mu sync.Mutex
	if err := downloadImages(ctx, cfg, cards, &mu); err != nil {
		return RunSummary{}, err
	}

	if err := writeCardsJSON(cfg.OutputJSONPath, cards); err != nil {
		return RunSummary{}, err
	}

	statusf("Done: wrote %s and %d WEBP images", cfg.OutputJSONPath, len(cards))
	return RunSummary{
		CardCount:      len(cards),
		OutputJSONPath: cfg.OutputJSONPath,
		ImagesDir:      cfg.ImagesDir,
	}, nil
}

func ListCards(ctx context.Context, cfg Config) ([]Card, error) {
	_, cards, cancel, err := prepareCards(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer cancel()

	listed := make([]Card, len(cards))
	copy(listed, cards)
	return listed, nil
}

func DownloadImagesOnly(ctx context.Context, cfg Config) error {
	_, err := RebuildImages(ctx, cfg)
	return err
}

func RebuildImages(ctx context.Context, cfg Config) (RunSummary, error) {
	cfg = withDefaults(cfg)
	if err := validateConfig(cfg); err != nil {
		return RunSummary{}, err
	}

	cards, err := loadCardsFromJSON(cfg.OutputJSONPath)
	if err != nil {
		return RunSummary{}, err
	}
	if len(cards) == 0 {
		return RunSummary{}, fmt.Errorf("no cards found in %s", cfg.OutputJSONPath)
	}

	for idx := range cards {
		setID := cards[idx].SetID
		if strings.TrimSpace(setID) == "" {
			setID = extractSetIDFromCardID(cards[idx].ID)
		}
		if strings.TrimSpace(setID) == "" {
			return RunSummary{}, fmt.Errorf("missing set_id for %s", cards[idx].ID)
		}
		cards[idx].SetID = setID
		cards[idx].SourceImageURL = buildSourceImageURL(cfg.SearchURL, setID, cards[idx].ID)
	}

	statusf("Removing existing images in %s", cfg.ImagesDir)
	if err := os.RemoveAll(cfg.ImagesDir); err != nil {
		return RunSummary{}, fmt.Errorf("remove images directory: %w", err)
	}
	if err := os.MkdirAll(cfg.ImagesDir, 0o755); err != nil {
		return RunSummary{}, fmt.Errorf("create images directory: %w", err)
	}

	statusf("Preparing %d cards for image download", len(cards))
	var mu sync.Mutex
	if err := downloadImages(ctx, cfg, cards, &mu); err != nil {
		return RunSummary{}, err
	}

	return RunSummary{
		CardCount:      len(cards),
		OutputJSONPath: cfg.OutputJSONPath,
		ImagesDir:      cfg.ImagesDir,
	}, nil
}

func prepareCards(ctx context.Context, cfg Config) (Config, []Card, context.CancelFunc, error) {
	cfg = withDefaults(cfg)
	if err := validateConfig(cfg); err != nil {
		return Config{}, nil, nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	cards, err := collectCards(ctx, cfg)
	if err != nil {
		cancel()
		return Config{}, nil, nil, err
	}

	return cfg, cards, cancel, nil
}

func withDefaults(cfg Config) Config {
	if strings.TrimSpace(cfg.SearchURL) == "" {
		cfg.SearchURL = DefaultSearchURL
	}
	if strings.TrimSpace(cfg.OutputJSONPath) == "" {
		cfg.OutputJSONPath = "output/cards.json"
	}
	if strings.TrimSpace(cfg.ImagesDir) == "" {
		cfg.ImagesDir = "output/images"
	}
	if strings.TrimSpace(cfg.UserAgent) == "" {
		cfg.UserAgent = "tcg-scout/1.0"
	}
	if cfg.WebPQuality == 0 {
		cfg.WebPQuality = 100
	}
	if cfg.ImageConcurrency == 0 {
		cfg.ImageConcurrency = 8
	}
	if cfg.PageConcurrency == 0 {
		cfg.PageConcurrency = 4
	}

	return cfg
}

func validateConfig(cfg Config) error {
	if strings.TrimSpace(cfg.SearchURL) == "" {
		return fmt.Errorf("search URL is required")
	}
	if cfg.WebPQuality < 0 || cfg.WebPQuality > 100 {
		return fmt.Errorf("the -webp-quality flag must be between 0 and 100")
	}
	if cfg.ImageConcurrency < 1 {
		return fmt.Errorf("the -image-concurrency flag must be at least 1")
	}
	if cfg.PageConcurrency < 1 {
		return fmt.Errorf("the -page-concurrency flag must be at least 1")
	}

	return nil
}

func collectCards(ctx context.Context, cfg Config) ([]Card, error) {
	searchURL, err := url.Parse(cfg.SearchURL)
	if err != nil {
		return nil, fmt.Errorf("parse search URL: %w", err)
	}

	textURL := buildTextViewURL(searchURL)
	bundle, err := fetchCardBundle(ctx, cfg, textURL)
	if err != nil {
		return nil, err
	}

	statusf("Found %d cards on page 1 and %d total pages", len(bundle.cards), bundle.maxPage)

	craftByID, err := collectCraftMappings(ctx, cfg, textURL)
	if err != nil {
		return nil, err
	}

	setID := extractSetID(textURL, bundle.cards)
	setName := bundle.setName
	if setName == "" {
		setName = setID
	}

	for idx := range bundle.cards {
		craftName, ok := craftByID[bundle.cards[idx].ID]
		if !ok {
			return nil, fmt.Errorf("missing craft mapping for %s", bundle.cards[idx].ID)
		}
		rarityID, ok := rarityIDs[bundle.cards[idx].Rarity]
		if !ok {
			return nil, fmt.Errorf("missing rarity mapping for %q", bundle.cards[idx].Rarity)
		}

		bundle.cards[idx].CraftName = craftName
		bundle.cards[idx].CraftID = craftIDs[craftName]
		bundle.cards[idx].RarityID = rarityID
		bundle.cards[idx].SetID = setID
		bundle.cards[idx].SetName = setName
		bundle.cards[idx].SetOrder = defaultSetOrder
		bundle.cards[idx].Language = defaultLanguage
	}

	return dedupeCards(bundle.cards), nil
}

func fetchCardBundle(ctx context.Context, cfg Config, textURL *url.URL) (pageBundle, error) {
	body, err := fetchPage(ctx, cfg, textURL.String())
	if err != nil {
		return pageBundle{}, fmt.Errorf("fetch search page: %w", err)
	}

	firstPage, err := parseTextSearchPage(textURL, body)
	if err != nil {
		return pageBundle{}, err
	}

	if firstPage.maxPage <= 1 {
		return firstPage, nil
	}

	extraCards, err := collectPaginatedCards(ctx, cfg, textURL, firstPage.maxPage)
	if err != nil {
		return pageBundle{}, err
	}

	firstPage.cards = append(firstPage.cards, extraCards...)
	firstPage.cards = dedupeCards(firstPage.cards)
	return firstPage, nil
}

func collectCraftMappings(ctx context.Context, cfg Config, textURL *url.URL) (map[string]string, error) {
	craftByID := make(map[string]string)

	for _, craftName := range craftNames {
		statusf("Mapping craft %s", craftName)
		craftURL := buildCraftTextURL(textURL, craftName)
		bundle, err := fetchCardBundle(ctx, cfg, craftURL)
		if err != nil {
			return nil, fmt.Errorf("collect craft mapping for %s: %w", craftName, err)
		}

		for _, card := range bundle.cards {
			craftByID[card.ID] = craftName
		}
	}

	return craftByID, nil
}

func collectPaginatedCards(ctx context.Context, cfg Config, textURL *url.URL, maxPage int) ([]Card, error) {
	byPage := make([][]Card, maxPage+1)
	var mu sync.Mutex

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(minInt(cfg.PageConcurrency, maxPage-1))

	for page := 2; page <= maxPage; page++ {
		page := page
		group.Go(func() error {
			pageURL, err := buildSearchResultsExURL(textURL, page)
			if err != nil {
				return err
			}

			statusf("Fetching page %d/%d", page, maxPage)
			body, err := fetchPage(groupCtx, cfg, pageURL)
			if err != nil {
				return fmt.Errorf("fetch search page %d: %w", page, err)
			}

			cards, err := parseTextSearchFragment(textURL, body)
			if err != nil {
				return fmt.Errorf("parse search page %d: %w", page, err)
			}

			mu.Lock()
			byPage[page] = cards
			mu.Unlock()
			statusf("Collected %d cards from page %d/%d", len(cards), page, maxPage)
			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, err
	}

	collected := make([]Card, 0)
	for page := 2; page <= maxPage; page++ {
		collected = append(collected, byPage[page]...)
	}

	return collected, nil
}

func downloadImages(ctx context.Context, cfg Config, cards []Card, mu *sync.Mutex) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	collector := newCollector(ctx, cfg, true)
	var (
		firstErr error
		errMu    sync.Mutex
	)

	collector.OnResponse(func(resp *colly.Response) {
		if err := ctx.Err(); err != nil {
			setFirstErr(&errMu, &firstErr, err)
			return
		}

		index, err := strconv.Atoi(resp.Ctx.Get("index"))
		if err != nil {
			setFirstErrCancel(&errMu, &firstErr, fmt.Errorf("parse image index: %w", err), cancel)
			return
		}

		img, _, err := image.Decode(bytes.NewReader(resp.Body))
		if err != nil {
			setFirstErrCancel(&errMu, &firstErr, fmt.Errorf("decode image for %s: %w", cards[index].ID, err), cancel)
			return
		}

		targetPath := filepath.Join(cfg.ImagesDir, buildImageName(cards[index].ID))
		file, err := os.Create(targetPath)
		if err != nil {
			setFirstErrCancel(&errMu, &firstErr, fmt.Errorf("create target image for %s: %w", cards[index].ID, err), cancel)
			return
		}
		if err := webp.Encode(file, img, &webp.Options{Quality: float32(cfg.WebPQuality)}); err != nil {
			_ = file.Close()
			setFirstErrCancel(&errMu, &firstErr, fmt.Errorf("encode WEBP for %s: %w", cards[index].ID, err), cancel)
			return
		}
		if err := file.Close(); err != nil {
			setFirstErrCancel(&errMu, &firstErr, fmt.Errorf("close WEBP file for %s: %w", cards[index].ID, err), cancel)
			return
		}

		mu.Lock()
		cards[index].ImageFile = filepath.ToSlash(targetPath)
		mu.Unlock()
	})
	collector.OnError(func(_ *colly.Response, err error) {
		setFirstErrCancel(&errMu, &firstErr, fmt.Errorf("download image: %w", err), cancel)
	})

	if cfg.ImageConcurrency > 1 {
		if err := collector.Limit(&colly.LimitRule{
			DomainRegexp: ".*",
			Parallelism:  cfg.ImageConcurrency,
		}); err != nil {
			return fmt.Errorf("configure image concurrency: %w", err)
		}
	}

	statusf("Downloading %d images with %d workers", len(cards), cfg.ImageConcurrency)

	for idx := range cards {
		if err := ctx.Err(); err != nil {
			break
		}
		if cards[idx].SourceImageURL == "" {
			continue
		}

		requestCtx := colly.NewContext()
		requestCtx.Put("index", strconv.Itoa(idx))
		if err := collector.Request(http.MethodGet, cards[idx].SourceImageURL, nil, requestCtx, nil); err != nil {
			setFirstErrCancel(&errMu, &firstErr, fmt.Errorf("queue image download for %s: %w", cards[idx].ID, err), cancel)
			break
		}
	}

	collector.Wait()
	return firstErr
}

func fetchPage(ctx context.Context, cfg Config, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", cfg.UserAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}

	return io.ReadAll(resp.Body)
}

func newCollector(ctx context.Context, cfg Config, async bool) *colly.Collector {
	options := []colly.CollectorOption{
		colly.StdlibContext(ctx),
		colly.UserAgent(cfg.UserAgent),
	}
	if async {
		options = append(options, colly.Async(true))
	}
	if domain := strings.TrimSpace(cfg.AllowedDomain); domain != "" {
		options = append(options, colly.AllowedDomains(domain))
	} else if parsed, err := url.Parse(cfg.SearchURL); err == nil && parsed.Hostname() != "" {
		options = append(options, colly.AllowedDomains(parsed.Hostname()))
	}

	return colly.NewCollector(options...)
}

func parseTextSearchPage(baseURL *url.URL, body []byte) (pageBundle, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return pageBundle{}, fmt.Errorf("parse search page HTML: %w", err)
	}

	cards := parseCardsFromSelection(baseURL, doc.Find("ul.cardlist-Result_List.cardlist-Result_List_Txt").First().ChildrenFiltered("li"))
	if len(cards) == 0 {
		return pageBundle{}, fmt.Errorf("no cards found in search page")
	}

	heading := normalizeInlineText(doc.Find("h2.cardlist-Result_Ttl").First().Text())

	return pageBundle{
		cards:   cards,
		maxPage: parseMaxPage(body),
		setName: parseSetName(heading),
	}, nil
}

func parseTextSearchFragment(baseURL *url.URL, body []byte) ([]Card, error) {
	markup := "<ul>" + string(body) + "</ul>"
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(markup))
	if err != nil {
		return nil, fmt.Errorf("parse search fragment HTML: %w", err)
	}

	return parseCardsFromSelection(baseURL, doc.Find("li")), nil
}

func parseCardsFromSelection(baseURL *url.URL, items *goquery.Selection) []Card {
	cards := make([]Card, 0, items.Length())

	items.Each(func(_ int, item *goquery.Selection) {
		link := item.Find("a").First()
		img := item.Find("img").First()

		detailURL := resolveURL(baseURL, link.AttrOr("href", ""))
		sourceImageURL := resolveURL(baseURL, img.AttrOr("src", ""))
		cardID := firstNonEmpty(
			normalizeInlineText(item.Find("p.number").First().Text()),
			extractCardNumber(detailURL),
		)
		name := firstNonEmpty(
			normalizeInlineText(item.Find("p.ttl").First().Text()),
			normalizeInlineText(img.AttrOr("title", img.AttrOr("alt", ""))),
		)

		if detailURL == "" || sourceImageURL == "" || cardID == "" || name == "" {
			return
		}

		mainType, subType := splitMainAndSubType(normalizeInlineText(item.Find("div.status span.status-Item").Eq(0).Text()))
		traits := splitValues(normalizeInlineText(item.Find("div.status span.status-Item").Eq(1).Text()))
		rarity := normalizeInlineText(item.Find("div.status span.status-Item").Eq(2).Text())
		effects := normalizeEffectHTML(item.Find("div.detail").First())
		effectText := normalizeMultilineText(selectionTextWithBreaks(item.Find("div.detail").First()))

		card := Card{
			ID:             cardID,
			Name:           name,
			Rarity:         rarity,
			MainType:       mainType,
			SubType:        subType,
			Abilities:      extractAbilities(effectText, effects),
			Effects:        effects,
			Traits:         traits,
			DetailURL:      detailURL,
			SourceImageURL: sourceImageURL,
		}

		item.Find("div.status span.status-Item").Each(func(_ int, status *goquery.Selection) {
			label := normalizeInlineText(status.Find("span.heading").First().Text())
			if label == "" {
				return
			}

			fullText := normalizeInlineText(status.Text())
			value := strings.TrimSpace(strings.TrimPrefix(fullText, label))
			switch label {
			case "Cost":
				card.Cost = parseNullableInt(value)
			case "Attack":
				card.Atk = parseNullableInt(value)
			case "Defense":
				card.Health = parseNullableInt(value)
			}
		})

		cards = append(cards, card)
	})

	return cards
}

func parseMaxPage(body []byte) int {
	matches := maxPagePattern.FindSubmatch(body)
	if len(matches) != 2 {
		return 1
	}

	maxPage, err := strconv.Atoi(string(matches[1]))
	if err != nil || maxPage < 1 {
		return 1
	}

	return maxPage
}

func parseSetName(heading string) string {
	matches := setNamePattern.FindStringSubmatch(heading)
	if len(matches) == 2 {
		return strings.TrimSpace(matches[1])
	}

	return heading
}

func buildTextViewURL(searchURL *url.URL) *url.URL {
	u := *searchURL
	query := u.Query()
	query.Set("view", "text")
	if query.Get("sort") == "" {
		query.Set("sort", "new")
	}
	u.RawQuery = query.Encode()
	return &u
}

func buildCraftTextURL(textURL *url.URL, craftName string) *url.URL {
	u := *textURL
	query := u.Query()
	query.Set("s_param", craftName)
	u.RawQuery = query.Encode()
	return &u
}

func buildSearchResultsExURL(searchURL *url.URL, page int) (string, error) {
	if page < 2 {
		return "", fmt.Errorf("extra search page must be at least 2")
	}

	u := *searchURL
	u.Path = strings.TrimSuffix(u.Path, "/")
	u.Path = strings.Replace(u.Path, "searchresults", "searchresults_ex", 1)

	query := u.Query()
	query.Set("view", "text")
	query.Set("page", strconv.Itoa(page))
	u.RawQuery = query.Encode()

	return u.String(), nil
}

func dedupeCards(cards []Card) []Card {
	seen := make(map[string]struct{}, len(cards))
	deduped := make([]Card, 0, len(cards))

	for _, card := range cards {
		if _, exists := seen[card.ID]; exists {
			continue
		}
		seen[card.ID] = struct{}{}
		deduped = append(deduped, card)
	}

	return deduped
}

func resolveURL(base *url.URL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}

	return base.ResolveReference(parsed).String()
}

func extractCardNumber(detailURL string) string {
	parsed, err := url.Parse(detailURL)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(parsed.Query().Get("cardno"))
}

func extractSetID(searchURL *url.URL, cards []Card) string {
	if setID := strings.TrimSpace(searchURL.Query().Get("expansion_name")); setID != "" {
		return setID
	}
	if len(cards) == 0 {
		return ""
	}

	cardID := cards[0].ID
	if dash := strings.IndexByte(cardID, '-'); dash > 0 {
		return cardID[:dash]
	}

	return cardID
}

func extractSetIDFromCardID(cardID string) string {
	if dash := strings.IndexByte(cardID, '-'); dash > 0 {
		return cardID[:dash]
	}

	return ""
}

func buildSourceImageURL(searchURLRaw, setID, cardID string) string {
	baseURL, err := url.Parse(searchURLRaw)
	if err != nil {
		return ""
	}

	imagePath := fmt.Sprintf("/wordpress/wp-content/images/cardlist/%s/%s.png", setID, cardID)
	return baseURL.ResolveReference(&url.URL{Path: imagePath}).String()
}

func buildImageName(cardID string) string {
	cardID = strings.TrimSpace(cardID)
	if cardID == "" {
		return "card.webp"
	}

	return cardID + ".webp"
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = nonSlugPattern.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	return value
}

func splitMainAndSubType(value string) (string, *string) {
	parts := splitValues(value)
	if len(parts) == 0 {
		return "", nil
	}
	if len(parts) == 1 {
		return parts[0], nil
	}

	subType := strings.Join(parts[1:], " / ")
	return parts[0], &subType
}

func splitValues(value string) []string {
	value = normalizeInlineText(value)
	if value == "" || value == "-" {
		return []string{}
	}

	parts := strings.Split(value, " / ")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = normalizeInlineText(part)
		if part == "" || part == "-" {
			continue
		}
		values = append(values, part)
	}

	return values
}

func normalizeInlineText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func normalizeMultilineText(value string) string {
	lines := strings.Split(value, "\n")
	normalized := make([]string, 0, len(lines))
	for _, line := range lines {
		line = normalizeInlineText(line)
		if line == "" {
			continue
		}
		normalized = append(normalized, line)
	}

	return strings.Join(normalized, "\n")
}

func normalizeEffectHTML(selection *goquery.Selection) string {
	htmlValue, err := selection.Html()
	if err != nil {
		return ""
	}

	htmlValue = strings.TrimSpace(htmlValue)
	htmlValue = effectIconPathPattern.ReplaceAllString(htmlValue, `/effect-icons/$1.png`)
	return htmlValue
}

func extractAbilities(effectText, effectHTML string) []string {
	seen := make(map[string]struct{})
	abilities := make([]string, 0)

	for _, match := range abilityTextPattern.FindAllString(effectText, -1) {
		ability := normalizeInlineText(match)
		if ability == "" {
			continue
		}
		if _, exists := seen[ability]; exists {
			continue
		}
		seen[ability] = struct{}{}
		abilities = append(abilities, ability)
	}

	for _, match := range effectIconAltPattern.FindAllStringSubmatch(effectHTML, -1) {
		if len(match) != 2 {
			continue
		}
		ability, ok := effectIconAbilities[strings.ToLower(match[1])]
		if !ok {
			continue
		}
		if _, exists := seen[ability]; exists {
			continue
		}
		seen[ability] = struct{}{}
		abilities = append(abilities, ability)
	}

	return abilities
}

func parseNullableInt(value string) *int {
	value = normalizeInlineText(value)
	if value == "" || value == "-" {
		return nil
	}

	number, err := strconv.Atoi(value)
	if err != nil {
		return nil
	}

	return &number
}

func selectionTextWithBreaks(selection *goquery.Selection) string {
	if selection.Length() == 0 {
		return ""
	}

	var builder strings.Builder
	for _, node := range selection.Nodes {
		writeNodeText(&builder, node)
	}

	return builder.String()
}

func writeNodeText(builder *strings.Builder, node *html.Node) {
	switch node.Type {
	case html.TextNode:
		builder.WriteString(node.Data)
	case html.ElementNode:
		if node.Data == "br" {
			builder.WriteByte('\n')
			return
		}
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		writeNodeText(builder, child)
	}
}

func setFirstErr(mu *sync.Mutex, target *error, err error) {
	if err == nil {
		return
	}

	mu.Lock()
	defer mu.Unlock()
	if *target == nil {
		*target = err
	}
}

func setFirstErrCancel(mu *sync.Mutex, target *error, err error, cancel context.CancelFunc) {
	setFirstErr(mu, target, err)
	if err != nil {
		cancel()
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}

	return ""
}

func statusf(format string, args ...interface{}) {
	slog.Info(fmt.Sprintf(format, args...))
}

func loadCardsFromJSON(path string) ([]Card, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read cards JSON: %w", err)
	}

	var cards []Card
	if err := json.Unmarshal(data, &cards); err != nil {
		return nil, fmt.Errorf("parse cards JSON: %w", err)
	}

	return cards, nil
}

func writeCardsJSON(path string, cards []Card) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create cards JSON: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(cards); err != nil {
		return fmt.Errorf("encode cards JSON: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close cards JSON: %w", err)
	}

	return nil
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
