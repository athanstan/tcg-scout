package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"tcg-scout/internal/api"
	"tcg-scout/internal/app"
	"tcg-scout/internal/scraper"
	"tcg-scout/internal/tcg/sve/decks"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type Streams struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

type commandSet struct {
	service   *app.Service
	streams   Streams
	buildInfo BuildInfo
}

func NewRootCommand(service *app.Service, streams Streams, buildInfo BuildInfo) *cobra.Command {
	commands := &commandSet{
		service:   service,
		streams:   streams,
		buildInfo: buildInfo,
	}

	commands.setDefaults()

	root := &cobra.Command{
		Use:           "tcg-scout",
		Short:         "Interactive CLI and API for TCG scraping workflows",
		Long:          "tcg-scout discovers games, resources, and scraper adapters, then runs them through a shared CLI and HTTP API.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return commands.initConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if commands.canPrompt(cmd) {
				return commands.runInteractive(cmd)
			}
			return cmd.Help()
		},
	}
	root.SetOut(streams.Out)
	root.SetErr(streams.Err)

	root.PersistentFlags().String("config", "", "config file path")
	root.PersistentFlags().String("log-level", "info", "log level (debug, info, warn, error)")
	root.PersistentFlags().String("output", "auto", "output format (auto, table, plain, json)")
	mustBindFlag("config", root.PersistentFlags().Lookup("config"))
	mustBindFlag("log-level", root.PersistentFlags().Lookup("log-level"))
	mustBindFlag("output", root.PersistentFlags().Lookup("output"))
	_ = root.RegisterFlagCompletionFunc("output", outputCompletion)

	root.AddCommand(commands.newGamesCommand())
	root.AddCommand(commands.newResourcesCommand())
	root.AddCommand(commands.newScrapersCommand())
	root.AddCommand(commands.newInteractiveCommand())
	root.AddCommand(commands.newServeCommand())
	root.AddCommand(commands.newVersionCommand())
	root.AddCommand(newCompletionCommand(root))

	for _, game := range service.Games() {
		root.AddCommand(commands.newGameCommand(game))
	}

	return root
}

func RewriteArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	if args[0] == "images-only" {
		return append([]string{"sve", "cards", "official", app.ActionImages}, args[1:]...)
	}
	if containsArg(args, "-names-only") {
		return append([]string{"sve", "cards", "official", app.ActionList}, removeArg(args, "-names-only")...)
	}
	if strings.HasPrefix(args[0], "-") {
		return append([]string{"sve", "cards", "official", app.ActionScrape}, args...)
	}
	if len(args) >= 3 && args[0] == "sve" && args[1] == "cards" {
		switch args[2] {
		case app.ActionScrape, app.ActionList, app.ActionImages:
			return append([]string{"sve", "cards", "official"}, args[2:]...)
		}
		if strings.HasPrefix(args[2], "-") {
			return append([]string{"sve", "cards", "official", app.ActionScrape}, args[2:]...)
		}
	}
	return args
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr interface{ ExitCode() int }
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	message := err.Error()
	if strings.Contains(message, "unknown command") ||
		strings.Contains(message, "unknown flag") ||
		strings.Contains(message, "accepts") ||
		strings.Contains(message, "requires at") ||
		strings.Contains(message, "required flag") ||
		strings.Contains(message, "unknown game") ||
		strings.Contains(message, "unknown resource") ||
		strings.Contains(message, "unknown scraper") ||
		strings.Contains(message, "unknown action") {
		return 2
	}
	return 1
}

func (c *commandSet) setDefaults() {
	request := app.DefaultRequest()
	viper.SetDefault("log-level", "info")
	viper.SetDefault("output", "auto")
	viper.SetDefault("search-url", request.SearchURL)
	viper.SetDefault("output-json", request.OutputJSONPath)
	viper.SetDefault("images-dir", request.ImagesDir)
	viper.SetDefault("user-agent", request.UserAgent)
	viper.SetDefault("allowed-domain", "")
	viper.SetDefault("webp-quality", request.WebPQuality)
	viper.SetDefault("image-concurrency", request.ImageConcurrency)
	viper.SetDefault("page-concurrency", request.PageConcurrency)
	viper.SetDefault("tournament-url", "")
	viper.SetDefault("output-dir", request.OutputDecksDir)
	viper.SetDefault("deck-concurrency", request.DeckConcurrency)
	viper.SetDefault("server-addr", ":8080")
	viper.SetDefault("server-read-timeout", "5s")
	viper.SetDefault("server-write-timeout", "30s")
	viper.SetDefault("server-idle-timeout", "60s")
	viper.SetDefault("server-shutdown-timeout", "10s")
}

func (c *commandSet) initConfig() error {
	if configPath := strings.TrimSpace(viper.GetString("config")); configPath != "" {
		viper.SetConfigFile(configPath)
	} else {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("find home directory: %w", err)
		}
		viper.AddConfigPath(homeDir)
		viper.AddConfigPath(".")
		viper.SetConfigName(".tcg-scout")
		viper.SetConfigType("yaml")
	}

	viper.SetEnvPrefix("TCG_SCOUT")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return fmt.Errorf("read config: %w", err)
		}
	}

	level := slog.LevelInfo
	switch strings.ToLower(viper.GetString("log-level")) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(c.streams.Err, &slog.HandlerOptions{Level: level})))
	return nil
}

func (c *commandSet) newGamesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "games",
		Short: "List supported games",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.writeGames(cmd, c.service.Games())
		},
	}
}

func (c *commandSet) newResourcesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "resources <game>",
		Short: "List supported resources for a game",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resources := c.service.Resources(args[0])
			if resources == nil {
				return fmt.Errorf("unknown game %q", args[0])
			}
			return c.writeResources(cmd, args[0], resources)
		},
	}
}

func (c *commandSet) newScrapersCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "scrapers <game> <resource>",
		Short: "List available scrapers for a game resource",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			scrapers := c.service.Scrapers(args[0], args[1])
			if scrapers == nil {
				return fmt.Errorf("unknown resource %q for game %q", args[1], args[0])
			}
			return c.writeScrapers(cmd, args[0], args[1], scrapers)
		},
	}
}

func (c *commandSet) newInteractiveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "interactive",
		Short: "Run a guided interactive prompt",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runInteractive(cmd)
		},
	}
}

func (c *commandSet) newServeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the shared scraper API",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			readTimeout, err := time.ParseDuration(viper.GetString("server-read-timeout"))
			if err != nil {
				return fmt.Errorf("parse read timeout: %w", err)
			}
			writeTimeout, err := time.ParseDuration(viper.GetString("server-write-timeout"))
			if err != nil {
				return fmt.Errorf("parse write timeout: %w", err)
			}
			idleTimeout, err := time.ParseDuration(viper.GetString("server-idle-timeout"))
			if err != nil {
				return fmt.Errorf("parse idle timeout: %w", err)
			}
			shutdownTimeout, err := time.ParseDuration(viper.GetString("server-shutdown-timeout"))
			if err != nil {
				return fmt.Errorf("parse shutdown timeout: %w", err)
			}

			server, err := api.NewServer(
				c.service,
				api.WithAddress(viper.GetString("server-addr")),
				api.WithReadTimeout(readTimeout),
				api.WithWriteTimeout(writeTimeout),
				api.WithIdleTimeout(idleTimeout),
				api.WithShutdownTimeout(shutdownTimeout),
			)
			if err != nil {
				return err
			}

			slog.Info("serving API", "addr", viper.GetString("server-addr"))
			return server.Run(cmd.Context())
		},
	}

	cmd.Flags().String("addr", ":8080", "API listen address")
	cmd.Flags().String("read-timeout", "5s", "HTTP read timeout")
	cmd.Flags().String("write-timeout", "30s", "HTTP write timeout")
	cmd.Flags().String("idle-timeout", "60s", "HTTP idle timeout")
	cmd.Flags().String("shutdown-timeout", "10s", "graceful shutdown timeout")
	mustBindFlag("server-addr", cmd.Flags().Lookup("addr"))
	mustBindFlag("server-read-timeout", cmd.Flags().Lookup("read-timeout"))
	mustBindFlag("server-write-timeout", cmd.Flags().Lookup("write-timeout"))
	mustBindFlag("server-idle-timeout", cmd.Flags().Lookup("idle-timeout"))
	mustBindFlag("server-shutdown-timeout", cmd.Flags().Lookup("shutdown-timeout"))

	return cmd
}

func (c *commandSet) newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build information",
		RunE: func(cmd *cobra.Command, args []string) error {
			version := c.buildInfo.Version
			if version == "" {
				version = "dev"
			}
			commit := c.buildInfo.Commit
			if commit == "" {
				commit = "unknown"
			}
			date := c.buildInfo.Date
			if date == "" {
				date = "unknown"
			}

			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "tcg-scout %s (commit: %s, built: %s)\n", version, commit, date); err != nil {
				return err
			}
			if info, ok := debug.ReadBuildInfo(); ok {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "go: %s\n", info.GoVersion)
				return err
			}
			return nil
		},
	}
}

func newCompletionCommand(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:       "completion [bash|zsh|fish|powershell]",
		Short:     "Generate shell completions",
		Args:      cobra.ExactValidArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return root.GenBashCompletionV2(cmd.OutOrStdout(), true)
			case "zsh":
				return root.GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return root.GenFishCompletion(cmd.OutOrStdout(), true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
			default:
				return nil
			}
		},
	}
}

func (c *commandSet) newGameCommand(game app.GameDefinition) *cobra.Command {
	cmd := &cobra.Command{
		Use:   game.ID,
		Short: game.Name,
		Long:  game.Summary,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.writeResources(cmd, game.ID, c.service.Resources(game.ID))
		},
	}

	for _, resource := range game.Resources {
		cmd.AddCommand(c.newResourceCommand(game, resource))
	}

	return cmd
}

func (c *commandSet) newResourceCommand(game app.GameDefinition, resource app.ResourceDefinition) *cobra.Command {
	cmd := &cobra.Command{
		Use:   resource.ID,
		Short: resource.Name,
		Long:  resource.Summary,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(resource.Scrapers) == 1 {
				return c.runAction(cmd, app.Selection{
					Game:     game.ID,
					Resource: resource.ID,
					Scraper:  resource.Scrapers[0].ID,
				}, resource.Scrapers[0].DefaultAction)
			}
			return c.writeScrapers(cmd, game.ID, resource.ID, c.service.Scrapers(game.ID, resource.ID))
		},
	}

	for _, scraperDefinition := range resource.Scrapers {
		cmd.AddCommand(c.newScraperCommand(game, resource, scraperDefinition))
	}

	return cmd
}

func (c *commandSet) newScraperCommand(game app.GameDefinition, resource app.ResourceDefinition, scraperDefinition app.ScraperDefinition) *cobra.Command {
	cmd := &cobra.Command{
		Use:   scraperDefinition.ID,
		Short: scraperDefinition.Name,
		Long:  scraperDefinition.Summary,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runAction(cmd, app.Selection{
				Game:     game.ID,
				Resource: resource.ID,
				Scraper:  scraperDefinition.ID,
			}, scraperDefinition.DefaultAction)
		},
	}

	for _, action := range scraperDefinition.Actions {
		action := action
		actionCmd := &cobra.Command{
			Use:   action.ID,
			Short: action.Summary,
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return c.runAction(cmd, app.Selection{
					Game:     game.ID,
					Resource: resource.ID,
					Scraper:  scraperDefinition.ID,
				}, action.ID)
			},
		}
		addRequestFlags(actionCmd, resource.ID)
		cmd.AddCommand(actionCmd)
	}

	return cmd
}

func addRequestFlags(cmd *cobra.Command, resourceID string) {
	cmd.Flags().String("user-agent", "tcg-scout/1.0", "HTTP user agent")
	mustBindFlag("user-agent", cmd.Flags().Lookup("user-agent"))

	switch resourceID {
	case "cards":
		cmd.Flags().String("search-url", scraper.DefaultSearchURL, "search results URL")
		cmd.Flags().String("output-json", "output/cards.json", "cards JSON output path")
		cmd.Flags().String("images-dir", "output/images", "WEBP image output directory")
		cmd.Flags().String("allowed-domain", "", "optional allowed domain")
		cmd.Flags().Float64("webp-quality", 100, "WEBP quality from 0 to 100")
		cmd.Flags().Int("image-concurrency", 8, "maximum concurrent image downloads")
		cmd.Flags().Int("page-concurrency", 4, "maximum concurrent paginated page fetches")

		mustBindFlag("search-url", cmd.Flags().Lookup("search-url"))
		mustBindFlag("output-json", cmd.Flags().Lookup("output-json"))
		mustBindFlag("images-dir", cmd.Flags().Lookup("images-dir"))
		mustBindFlag("allowed-domain", cmd.Flags().Lookup("allowed-domain"))
		mustBindFlag("webp-quality", cmd.Flags().Lookup("webp-quality"))
		mustBindFlag("image-concurrency", cmd.Flags().Lookup("image-concurrency"))
		mustBindFlag("page-concurrency", cmd.Flags().Lookup("page-concurrency"))
	case "decks":
		cmd.Flags().String("tournament-url", "", "official tournament deck list URL")
		cmd.Flags().String("output-dir", decks.DefaultOutputDir, "deck JSON output directory")
		cmd.Flags().Int("deck-concurrency", decks.DefaultDeckConcurrency, "maximum concurrent decklog fetches")

		mustBindFlag("tournament-url", cmd.Flags().Lookup("tournament-url"))
		mustBindFlag("output-dir", cmd.Flags().Lookup("output-dir"))
		mustBindFlag("deck-concurrency", cmd.Flags().Lookup("deck-concurrency"))
	}
}

func (c *commandSet) runAction(cmd *cobra.Command, selection app.Selection, action string) error {
	if err := viper.BindPFlags(cmd.Flags()); err != nil {
		return fmt.Errorf("bind flags: %w", err)
	}

	result, err := c.service.Execute(cmd.Context(), selection, action, c.buildRequest())
	if err != nil {
		return err
	}

	switch action {
	case app.ActionList:
		if len(result.Tournament) > 0 {
			return c.writeTournament(cmd, result.Tournament)
		}
		return c.writeCards(cmd, result.Cards)
	default:
		return c.writeSummary(cmd, result.Summary)
	}
}

func (c *commandSet) buildRequest() app.Request {
	return app.Request{
		SearchURL:        viper.GetString("search-url"),
		OutputJSONPath:   viper.GetString("output-json"),
		ImagesDir:        viper.GetString("images-dir"),
		UserAgent:        viper.GetString("user-agent"),
		AllowedDomain:    viper.GetString("allowed-domain"),
		WebPQuality:      viper.GetFloat64("webp-quality"),
		ImageConcurrency: viper.GetInt("image-concurrency"),
		PageConcurrency:  viper.GetInt("page-concurrency"),
		TournamentURL:    viper.GetString("tournament-url"),
		OutputDecksDir:     viper.GetString("output-dir"),
		DeckConcurrency:  viper.GetInt("deck-concurrency"),
	}
}

func (c *commandSet) runInteractive(cmd *cobra.Command) error {
	games := c.service.Games()
	if len(games) == 0 {
		return fmt.Errorf("no games registered")
	}

	reader := bufio.NewReader(c.streams.In)
	gameIndex, err := promptChoice(reader, cmd.OutOrStdout(), "Choose a game", gameOptions(games))
	if err != nil {
		return err
	}
	game := games[gameIndex]

	resources := c.service.Resources(game.ID)
	resourceIndex, err := promptChoice(reader, cmd.OutOrStdout(), "Choose a resource", resourceOptions(resources))
	if err != nil {
		return err
	}
	resource := resources[resourceIndex]

	scrapers := c.service.Scrapers(game.ID, resource.ID)
	scraperIndex, err := promptChoice(reader, cmd.OutOrStdout(), "Choose a scraper", scraperOptions(scrapers))
	if err != nil {
		return err
	}
	scraperDefinition := scrapers[scraperIndex]

	actionIndex, err := promptChoice(reader, cmd.OutOrStdout(), "Choose an action", actionOptions(scraperDefinition.Actions))
	if err != nil {
		return err
	}

	return c.runAction(cmd, app.Selection{
		Game:     game.ID,
		Resource: resource.ID,
		Scraper:  scraperDefinition.ID,
	}, scraperDefinition.Actions[actionIndex].ID)
}

func promptChoice(reader *bufio.Reader, out io.Writer, label string, options []option) (int, error) {
	if _, err := fmt.Fprintf(out, "%s:\n", label); err != nil {
		return 0, err
	}
	for index, option := range options {
		if _, err := fmt.Fprintf(out, "  %d. %s - %s\n", index+1, option.label, option.summary); err != nil {
			return 0, err
		}
	}
	if _, err := fmt.Fprint(out, "> "); err != nil {
		return 0, err
	}

	line, err := reader.ReadString('\n')
	if err != nil {
		return 0, err
	}
	choice, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || choice < 1 || choice > len(options) {
		return 0, fmt.Errorf("invalid choice %q", strings.TrimSpace(line))
	}

	return choice - 1, nil
}

func (c *commandSet) writeGames(cmd *cobra.Command, games []app.GameDefinition) error {
	format := c.resolveOutput(cmd, true)
	switch format {
	case "json":
		return writeJSON(cmd.OutOrStdout(), map[string]any{"games": games})
	case "plain":
		for _, game := range games {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", game.ID, game.Name); err != nil {
				return err
			}
		}
		return nil
	default:
		writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		if _, err := fmt.Fprintln(writer, "GAME\tNAME\tSUMMARY"); err != nil {
			return err
		}
		for _, game := range games {
			if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\n", game.ID, game.Name, game.Summary); err != nil {
				return err
			}
		}
		return writer.Flush()
	}
}

func (c *commandSet) writeResources(cmd *cobra.Command, gameID string, resources []app.ResourceDefinition) error {
	format := c.resolveOutput(cmd, true)
	switch format {
	case "json":
		return writeJSON(cmd.OutOrStdout(), map[string]any{"game": gameID, "resources": resources})
	case "plain":
		for _, resource := range resources {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", resource.ID, resource.Name); err != nil {
				return err
			}
		}
		return nil
	default:
		writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		if _, err := fmt.Fprintln(writer, "RESOURCE\tNAME\tSUMMARY"); err != nil {
			return err
		}
		for _, resource := range resources {
			if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\n", resource.ID, resource.Name, resource.Summary); err != nil {
				return err
			}
		}
		return writer.Flush()
	}
}

func (c *commandSet) writeScrapers(cmd *cobra.Command, gameID, resourceID string, scrapers []app.ScraperDefinition) error {
	format := c.resolveOutput(cmd, true)
	switch format {
	case "json":
		return writeJSON(cmd.OutOrStdout(), map[string]any{"game": gameID, "resource": resourceID, "scrapers": scrapers})
	case "plain":
		for _, scraperDefinition := range scrapers {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", scraperDefinition.ID, scraperDefinition.Name); err != nil {
				return err
			}
		}
		return nil
	default:
		writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		if _, err := fmt.Fprintln(writer, "SCRAPER\tNAME\tDEFAULT ACTION\tSUMMARY"); err != nil {
			return err
		}
		for _, scraperDefinition := range scrapers {
			if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", scraperDefinition.ID, scraperDefinition.Name, scraperDefinition.DefaultAction, scraperDefinition.Summary); err != nil {
				return err
			}
		}
		return writer.Flush()
	}
}

func (c *commandSet) writeTournament(cmd *cobra.Command, payload []byte) error {
	format := c.resolveOutput(cmd, false)
	if format != "json" {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "tournament data available with --output json")
		return err
	}

	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return err
	}
	return writeJSON(cmd.OutOrStdout(), value)
}

func (c *commandSet) writeCards(cmd *cobra.Command, cards []scraper.Card) error {
	format := c.resolveOutput(cmd, true)
	switch format {
	case "json":
		return writeJSON(cmd.OutOrStdout(), map[string]any{"cards": cards})
	case "plain":
		for _, card := range cards {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", card.ID, card.Name); err != nil {
				return err
			}
		}
		return nil
	default:
		writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		if _, err := fmt.Fprintln(writer, "ID\tNAME\tRARITY\tTYPE\tSET"); err != nil {
			return err
		}
		for _, card := range cards {
			if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", card.ID, card.Name, card.Rarity, card.MainType, card.SetID); err != nil {
				return err
			}
		}
		return writer.Flush()
	}
}

func (c *commandSet) writeSummary(cmd *cobra.Command, summary app.Summary) error {
	if c.resolveOutput(cmd, false) == "json" {
		return writeJSON(cmd.OutOrStdout(), summary)
	}
	_, err := fmt.Fprintln(cmd.OutOrStdout(), summary.Message)
	return err
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func (c *commandSet) resolveOutput(cmd *cobra.Command, allowTable bool) string {
	format := strings.ToLower(strings.TrimSpace(viper.GetString("output")))
	if format == "" || format == "auto" {
		if allowTable && c.isTerminal(cmd.OutOrStdout()) {
			return "table"
		}
		return "plain"
	}
	if !allowTable && format == "table" {
		return "plain"
	}
	return format
}

func (c *commandSet) canPrompt(cmd *cobra.Command) bool {
	return c.isTerminal(c.streams.In) && c.isTerminal(cmd.OutOrStdout())
}

func (c *commandSet) isTerminal(value any) bool {
	file, ok := value.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func outputCompletion(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return []string{
		"auto\tAuto-select table for terminals, plain otherwise",
		"table\tHuman-friendly table output",
		"plain\tTab-delimited plain output",
		"json\tMachine-readable JSON output",
	}, cobra.ShellCompDirectiveNoFileComp
}

func mustBindFlag(key string, flag *pflag.Flag) {
	if err := viper.BindPFlag(key, flag); err != nil {
		panic(err)
	}
}

type option struct {
	label   string
	summary string
}

func gameOptions(games []app.GameDefinition) []option {
	options := make([]option, 0, len(games))
	for _, game := range games {
		options = append(options, option{label: fmt.Sprintf("%s (%s)", game.Name, game.ID), summary: game.Summary})
	}
	return options
}

func resourceOptions(resources []app.ResourceDefinition) []option {
	options := make([]option, 0, len(resources))
	for _, resource := range resources {
		options = append(options, option{label: fmt.Sprintf("%s (%s)", resource.Name, resource.ID), summary: resource.Summary})
	}
	return options
}

func scraperOptions(scrapers []app.ScraperDefinition) []option {
	options := make([]option, 0, len(scrapers))
	for _, scraperDefinition := range scrapers {
		options = append(options, option{label: fmt.Sprintf("%s (%s)", scraperDefinition.Name, scraperDefinition.ID), summary: scraperDefinition.Summary})
	}
	return options
}

func actionOptions(actions []app.ActionDefinition) []option {
	options := make([]option, 0, len(actions))
	for _, action := range actions {
		options = append(options, option{label: action.ID, summary: action.Summary})
	}
	return options
}

func containsArg(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}

func removeArg(args []string, target string) []string {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == target {
			continue
		}
		filtered = append(filtered, arg)
	}
	return filtered
}
