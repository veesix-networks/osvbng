package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/chzyer/readline"
)

type CLI struct {
	api             *APIClient
	serverAddr      string
	rl              *readline.Instance
	running         bool
	formatter       *GenericFormatter
	contract        *Contract
	currentLine     string
	configMode      bool
	configSessionID string
}

type showResponseEnvelope struct {
	Path string      `json:"path"`
	Data interface{} `json:"data"`
}

type operationResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Version int    `json:"version,omitempty"`
}

func NewCLI(serverAddr string) (*CLI, error) {
	api, err := newAPIClient(serverAddr)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	contract, err := api.loadContract(ctx)
	if err != nil {
		return nil, err
	}

	return &CLI{
		api:        api,
		serverAddr: api.baseURLString(),
		running:    true,
		formatter:  NewGenericFormatter(),
		contract:   contract,
	}, nil
}

func (c *CLI) Run() error {
	var err error
	c.rl, err = readline.NewEx(&readline.Config{
		Prompt:              getPrompt(false),
		HistoryFile:         os.ExpandEnv("$HOME/.bngcli_history"),
		AutoComplete:        c.buildCompleter(),
		InterruptPrompt:     "^C",
		EOFPrompt:           "exit",
		FuncFilterInputRune: c.filterInputWithHelp,
		Listener:            c,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize readline: %w", err)
	}
	defer c.rl.Close()

	c.printBanner()

	for c.running {
		c.rl.SetPrompt(getPrompt(c.configMode))

		line, err := c.rl.Readline()
		if err != nil {
			if err == readline.ErrInterrupt {
				if len(line) == 0 {
					break
				}
				continue
			}
			if err == io.EOF {
				if c.configMode {
					_ = c.bestEffortDiscard()
				}
				break
			}
			return err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if err := c.processCommand(line); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
	}

	return nil
}

func (c *CLI) Stop() {
	c.running = false
}

func (c *CLI) OnChange(line []rune, pos int, key rune) (newLine []rune, newPos int, ok bool) {
	c.currentLine = string(line)
	return nil, 0, false
}

func (c *CLI) FormatOutput(data interface{}, format string) (string, error) {
	return c.formatter.Format(data, OutputFormat(format))
}

func (c *CLI) printBanner() {
	fmt.Println("=====================================")
	fmt.Println("    osvbng Interactive CLI")
	fmt.Println("=====================================")
	fmt.Printf("Connected to: %s\n", c.serverAddr)
	fmt.Println("Type 'help' for available commands")
	fmt.Println("Type 'exit' or 'quit' to exit")
	fmt.Println()
}

func (c *CLI) filterInputWithHelp(r rune) (rune, bool) {
	if r == '?' {
		fmt.Print("?\n")
		c.showHelpForInput(c.currentLine)
		if c.rl != nil {
			c.rl.Write([]byte(c.currentLine))
		}
		return 0, false
	}
	return filterInput(r)
}

func (c *CLI) processCommand(line string) error {
	if line == "?" {
		c.showHelpForInput("")
		return nil
	}

	if strings.HasSuffix(line, "?") {
		c.showHelpForInput(strings.TrimSuffix(line, "?"))
		return nil
	}

	invocation, err := parseInvocation(line)
	if err != nil {
		return err
	}
	if len(invocation.PathTokens) == 0 {
		return nil
	}

	if contains(invocation.FlagTokens, "--help") {
		c.showHelpForInput(strings.Join(invocation.PathTokens, " "))
		return nil
	}

	if err := c.handleBuiltin(invocation); err != nil {
		if err == errNotBuiltin {
			return c.executeGeneratedCommand(invocation)
		}
		return err
	}

	return nil
}

var errNotBuiltin = fmt.Errorf("not a builtin command")

func (c *CLI) handleBuiltin(invocation *Invocation) error {
	switch invocation.PathTokens[0] {
	case "help":
		c.showHelpForInput(strings.Join(invocation.PathTokens[1:], " "))
		return nil
	case "exit", "quit":
		if c.configMode {
			_ = c.bestEffortDiscard()
		}
		c.running = false
		return nil
	case "configure":
		if len(invocation.PathTokens) != 1 || len(invocation.FlagTokens) != 0 {
			return fmt.Errorf("usage: configure")
		}
		return c.enterConfigMode()
	case "commit":
		if len(invocation.PathTokens) != 1 || len(invocation.FlagTokens) != 0 {
			return fmt.Errorf("usage: commit")
		}
		return c.commitConfig()
	case "discard":
		if len(invocation.PathTokens) != 1 || len(invocation.FlagTokens) != 0 {
			return fmt.Errorf("usage: discard")
		}
		return c.discardConfig()
	default:
		return errNotBuiltin
	}
}

func (c *CLI) enterConfigMode() error {
	if c.configMode {
		return fmt.Errorf("already in configuration mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var response struct {
		SessionID string `json:"session_id"`
	}
	if err := c.api.doJSON(ctx, "POST", "/api/config/session", nil, nil, &response); err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return fmt.Errorf("enter configuration mode: server does not support /api/config/session; rebuild or restart osvbng on %s", c.serverAddr)
		}
		return fmt.Errorf("enter configuration mode: %w", err)
	}

	c.configMode = true
	c.configSessionID = response.SessionID
	fmt.Println("Entered configuration mode")
	fmt.Println("Use 'commit' to apply changes, 'discard' to cancel, 'exit' or 'quit' to leave the shell")
	return nil
}

func (c *CLI) commitConfig() error {
	if !c.configMode {
		return fmt.Errorf("not in configuration mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var response operationResponse
	path := "/api/config/session/" + url.PathEscape(c.configSessionID) + "/commit"
	if err := c.api.doJSON(ctx, "POST", path, nil, nil, &response); err != nil {
		return fmt.Errorf("commit configuration: %w", err)
	}

	c.configMode = false
	c.configSessionID = ""
	if response.Version > 0 {
		fmt.Printf("Configuration committed (version %d)\n", response.Version)
	} else {
		fmt.Println("Configuration committed")
	}
	return nil
}

func (c *CLI) discardConfig() error {
	if !c.configMode {
		return fmt.Errorf("not in configuration mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	path := "/api/config/session/" + url.PathEscape(c.configSessionID) + "/discard"
	if err := c.api.doJSON(ctx, "POST", path, nil, nil, &operationResponse{}); err != nil {
		return fmt.Errorf("discard configuration: %w", err)
	}

	c.configMode = false
	c.configSessionID = ""
	fmt.Println("Configuration changes discarded")
	return nil
}

func (c *CLI) bestEffortDiscard() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	path := "/api/config/session/" + url.PathEscape(c.configSessionID) + "/discard"
	err := c.api.doJSON(ctx, "POST", path, nil, nil, &operationResponse{})
	c.configMode = false
	c.configSessionID = ""
	return err
}

func (c *CLI) executeGeneratedCommand(invocation *Invocation) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	command, pathParams, positionalValue, err := c.contract.matchCommandForInvocation(invocation.PathTokens)
	if err != nil {
		return err
	}

	query, body, err := command.parseFlags(invocation.FlagTokens, positionalValue)
	if err != nil {
		return err
	}

	switch command.Kind {
	case CommandShow:
		var response showResponseEnvelope
		if err := c.api.doJSON(ctx, command.Method, command.apiCommandPath(pathParams), query, nil, &response); err != nil {
			return err
		}
		return c.printFormatted(response.Data, invocation.Format)
	case CommandExec:
		var response interface{}
		if err := c.api.doJSON(ctx, command.Method, command.apiCommandPath(pathParams), query, body, &response); err != nil {
			return err
		}
		return c.printFormatted(response, invocation.Format)
	case CommandSet:
		return c.executeSetCommand(ctx, command, pathParams, query, body, invocation.Format)
	default:
		return fmt.Errorf("unsupported command kind %q", command.Kind)
	}
}

func (c *CLI) executeSetCommand(ctx context.Context, command *GeneratedCommand, pathParams map[string]string, query url.Values, body interface{}, format OutputFormat) error {
	var (
		response operationResponse
		path     string
	)

	if c.configMode {
		path = "/api/config/session/" + url.PathEscape(c.configSessionID) + "/set/" + strings.TrimPrefix(command.apiCommandPath(pathParams), "/api/set/")
	} else {
		path = command.apiCommandPath(pathParams)
	}

	if err := c.api.doJSON(ctx, "POST", path, query, body, &response); err != nil {
		return err
	}

	if format == FormatCLI {
		fmt.Println("OK")
		return nil
	}

	return c.printFormatted(response, format)
}

func (c *CLI) printFormatted(data interface{}, format OutputFormat) error {
	output, err := c.formatter.Format(data, format)
	if err != nil {
		return err
	}
	fmt.Print(output)
	return nil
}

func (c *CLI) showHelpForInput(input string) {
	input = strings.TrimLeft(input, " \t\r\n")
	if strings.TrimSpace(input) == "" {
		c.printSuggestions(c.rootSuggestions())
		return
	}
	endsWithSpace := strings.HasSuffix(input, " ")

	invocation, err := parseInvocation(input)
	if err != nil {
		fmt.Printf("\n  %v\n\n", err)
		return
	}

	if len(invocation.PathTokens) > 0 && invocation.PathTokens[0] == "help" {
		invocation, _ = parseInvocation(strings.Join(invocation.PathTokens[1:], " "))
	}

	if len(invocation.PathTokens) == 0 {
		c.printSuggestions(c.rootSuggestions())
		return
	}

	if endsWithSpace {
		suggestions := c.suggestionsForInput(input, true)
		if len(suggestions) == 0 {
			fmt.Print("\n  <cr>\n\n")
			return
		}
		c.printSuggestions(suggestions)
		return
	}

	command, _, err := c.contract.matchCommand(invocation.PathTokens)
	if err == nil {
		c.printCommandHelp(command)
		return
	}

	suggestions := c.suggestionsForInput(input, true)
	if len(suggestions) > 0 && c.hasDeeperPathPrefix(invocation.PathTokens) {
		descendantSuggestions := c.suggestionsForInput(input+" ", true)
		if len(descendantSuggestions) > 0 {
			suggestions = descendantSuggestions
		}
	}
	if len(suggestions) == 0 {
		fmt.Print("\n  <cr>\n\n")
		return
	}
	c.printSuggestions(suggestions)
}

func (c *CLI) printCommandHelp(command *GeneratedCommand) {
	fmt.Println()
	if command.Summary != "" {
		fmt.Printf("  %s\n", command.Summary)
	}
	if command.Description != "" {
		fmt.Printf("  %s\n", command.Description)
	}
	fmt.Printf("  Usage: %s\n", c.commandUsage(command))

	flags := make([]*FlagSpec, 0, len(command.allFlags()))
	for _, flag := range command.allFlags() {
		if flag == command.positionalScalarFlag() {
			continue
		}
		flags = append(flags, flag)
	}
	if len(flags) > 0 {
		fmt.Println()
		for _, flag := range flags {
			requirement := "optional"
			if flag.Required {
				requirement = "required"
			}
			desc := strings.TrimSpace(flag.Description)
			if desc == "" {
				desc = requirement
			} else {
				desc = desc + " (" + requirement + ")"
			}
			fmt.Printf("  --%-20s %s\n", flag.CLIName, desc)
		}
	}
	fmt.Println()
}

func (c *CLI) commandUsage(command *GeneratedCommand) string {
	parts := make([]string, 0, len(command.Segments))
	for _, segment := range command.Segments {
		if segment.Param != nil {
			parts = append(parts, formatPlaceholder(segment.Param.Name))
			continue
		}
		parts = append(parts, segment.Literal)
	}

	for _, flag := range command.allFlags() {
		if flag == command.positionalScalarFlag() {
			continue
		}
		valuePlaceholder := "<value>"
		if flag.Kind == ValueBoolean {
			valuePlaceholder = "<true|false>"
		}
		part := fmt.Sprintf("--%s %s", flag.CLIName, valuePlaceholder)
		if !flag.Required {
			part = "[" + part + "]"
		}
		parts = append(parts, part)
	}
	if flag := command.positionalScalarFlag(); flag != nil {
		parts = append(parts, formatValuePlaceholder(flag))
	}

	return strings.Join(parts, " ")
}

func (c *CLI) rootSuggestions() []Suggestion {
	suggestions := make([]Suggestion, 0)
	for _, builtin := range builtinCommands {
		suggestions = append(suggestions, Suggestion{
			Text:        builtin.Name,
			Description: builtin.Description,
		})
	}

	seen := make(map[string]bool)
	for _, command := range c.contract.Commands {
		if len(command.Segments) == 0 || command.Segments[0].Literal == "" {
			continue
		}
		name := command.Segments[0].Literal
		if seen[name] {
			continue
		}
		seen[name] = true
		suggestions = append(suggestions, Suggestion{
			Text:        name,
			Description: string(command.Kind) + " commands",
		})
	}

	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Text < suggestions[j].Text
	})
	return suggestions
}

func (c *CLI) suggestionsForInput(input string, includePlaceholders bool) []Suggestion {
	tokens, _ := tokenizeLine(input, true)
	endsWithSpace := strings.HasSuffix(input, " ")

	pipeIndex := -1
	for i, token := range tokens {
		if token == "|" {
			pipeIndex = i
			break
		}
	}
	if pipeIndex >= 0 {
		return c.formatSuggestions(tokens, pipeIndex, endsWithSpace)
	}

	if len(tokens) == 0 {
		return c.rootSuggestions()
	}

	if tokens[0] == "help" {
		remaining := strings.TrimSpace(strings.TrimPrefix(input, "help"))
		return c.suggestionsForInput(strings.TrimSpace(remaining), includePlaceholders)
	}

	pathEnd := len(tokens)
	for i, token := range tokens {
		if strings.HasPrefix(token, "--") {
			pathEnd = i
			break
		}
	}

	pathTokens := tokens[:pathEnd]
	flagTokens := tokens[pathEnd:]

	if len(flagTokens) > 0 {
		return c.flagSuggestions(pathTokens, flagTokens, endsWithSpace)
	}

	return c.pathSuggestions(pathTokens, endsWithSpace, includePlaceholders)
}

func (c *CLI) formatSuggestions(tokens []string, pipeIndex int, endsWithSpace bool) []Suggestion {
	partial := ""
	if !endsWithSpace && pipeIndex+1 < len(tokens) {
		partial = tokens[len(tokens)-1]
	}

	var suggestions []Suggestion
	for _, format := range []string{"cli", "json", "yaml"} {
		if partial == "" || strings.HasPrefix(format, partial) {
			suggestions = append(suggestions, Suggestion{Text: format})
		}
	}
	return suggestions
}

func (c *CLI) pathSuggestions(pathTokens []string, endsWithSpace bool, includePlaceholders bool) []Suggestion {
	if len(pathTokens) == 0 {
		return c.rootSuggestions()
	}

	completed := pathTokens
	partial := ""
	if !endsWithSpace {
		completed = pathTokens[:len(pathTokens)-1]
		partial = pathTokens[len(pathTokens)-1]
	}

	var suggestions []Suggestion
	if len(completed) == 0 {
		for _, builtin := range builtinCommands {
			if partial == "" || strings.HasPrefix(builtin.Name, partial) {
				suggestions = append(suggestions, Suggestion{Text: builtin.Name, Description: builtin.Description})
			}
		}
	}

	for _, command := range c.contract.Commands {
		positionalFlag := command.positionalScalarFlag()

		if positionalFlag != nil && len(pathTokens) == len(command.Segments)+1 {
			prefixTokens := pathTokens[:len(command.Segments)]
			if !prefixMatches(command.Segments, prefixTokens) {
				continue
			}
			valueToken := pathTokens[len(pathTokens)-1]
			if endsWithSpace {
				suggestions = append(suggestions, Suggestion{Text: "|", Description: "Select output format"})
				continue
			}
			if len(positionalFlag.Enum) > 0 {
				suggestions = append(suggestions, c.flagValueSuggestions(positionalFlag, valueToken)...)
			}
			continue
		}

		if !prefixMatches(command.Segments, completed) {
			continue
		}
		if len(completed) >= len(command.Segments) {
			if endsWithSpace {
				if positionalFlag != nil {
					if len(positionalFlag.Enum) > 0 {
						suggestions = append(suggestions, c.flagValueSuggestions(positionalFlag, "")...)
						continue
					}
					if includePlaceholders {
						suggestions = append(suggestions, Suggestion{
							Text:        formatValuePlaceholder(positionalFlag),
							Description: strings.TrimSpace(positionalFlag.Description),
						})
					}
				} else {
					suggestions = append(suggestions, c.commandFlagSuggestions(command, nil, "")...)
					suggestions = append(suggestions, Suggestion{Text: "|", Description: "Select output format"})
				}
			}
			continue
		}

		next := command.Segments[len(completed)]
		if next.Param != nil {
			if includePlaceholders && partial == "" {
				suggestions = append(suggestions, Suggestion{
					Text:        formatPlaceholder(next.Param.Name),
					Description: strings.TrimSpace(next.Param.Description),
				})
			}
			continue
		}

		if partial == "" || strings.HasPrefix(next.Literal, partial) {
			suggestions = append(suggestions, Suggestion{
				Text:        next.Literal,
				Description: strings.TrimSpace(command.Summary),
			})
		}
	}

	return uniqueSuggestions(suggestions)
}

func (c *CLI) flagSuggestions(pathTokens []string, flagTokens []string, endsWithSpace bool) []Suggestion {
	command, _, err := c.contract.matchCommand(pathTokens)
	if err != nil {
		return nil
	}

	if len(flagTokens) == 0 {
		return c.commandFlagSuggestions(command, nil, "")
	}

	last := flagTokens[len(flagTokens)-1]
	flagMap := make(map[string]*FlagSpec)
	for _, flag := range command.allFlags() {
		flagMap["--"+flag.CLIName] = flag
	}

	if endsWithSpace {
		if flag := flagMap[last]; flag != nil {
			return c.flagValueSuggestions(flag, "")
		}
		return c.commandFlagSuggestions(command, flagTokens, "")
	}

	if len(flagTokens) >= 2 {
		if flag := flagMap[flagTokens[len(flagTokens)-2]]; flag != nil {
			return c.flagValueSuggestions(flag, last)
		}
	}

	if strings.HasPrefix(last, "--") {
		return c.commandFlagSuggestions(command, flagTokens[:len(flagTokens)-1], last)
	}

	return nil
}

func (c *CLI) commandFlagSuggestions(command *GeneratedCommand, existing []string, partial string) []Suggestion {
	used := make(map[string]bool)
	for _, token := range existing {
		if strings.HasPrefix(token, "--") {
			used[token] = true
		}
	}

	suggestions := make([]Suggestion, 0)
	for _, flag := range command.allFlags() {
		if flag == command.positionalScalarFlag() {
			continue
		}
		name := "--" + flag.CLIName
		if used[name] && !flag.Repeated {
			continue
		}
		if partial == "" || strings.HasPrefix(name, partial) {
			suggestions = append(suggestions, Suggestion{
				Text:        name,
				Description: strings.TrimSpace(flag.Description),
			})
		}
	}
	suggestions = append(suggestions, Suggestion{Text: "|", Description: "Select output format"})
	return uniqueSuggestions(suggestions)
}

func (c *CLI) flagValueSuggestions(flag *FlagSpec, partial string) []Suggestion {
	if len(flag.Enum) == 0 {
		return nil
	}

	suggestions := make([]Suggestion, 0, len(flag.Enum))
	for _, value := range flag.Enum {
		if partial == "" || strings.HasPrefix(value, partial) {
			suggestions = append(suggestions, Suggestion{Text: value})
		}
	}
	return suggestions
}

func formatValuePlaceholder(flag *FlagSpec) string {
	if flag == nil {
		return "<value>"
	}
	if flag.Kind == ValueBoolean {
		return "<true|false>"
	}
	if len(flag.Enum) > 0 {
		return "<" + strings.Join(flag.Enum, "|") + ">"
	}
	return "<value>"
}

func (c *CLI) printSuggestions(suggestions []Suggestion) {
	if len(suggestions) == 0 {
		fmt.Print("\n  <cr>\n\n")
		return
	}

	fmt.Println()
	for _, suggestion := range suggestions {
		if suggestion.Description != "" {
			fmt.Printf("  %-20s %s\n", suggestion.Text, suggestion.Description)
		} else {
			fmt.Printf("  %s\n", suggestion.Text)
		}
	}
	fmt.Println()
}

func prefixMatches(segments []CommandSegment, completed []string) bool {
	if len(completed) > len(segments) {
		return false
	}

	for i, token := range completed {
		if segments[i].Param != nil {
			continue
		}
		if segments[i].Literal != token {
			return false
		}
	}
	return true
}

func (c *CLI) hasDeeperPathPrefix(pathTokens []string) bool {
	for _, command := range c.contract.Commands {
		if len(command.Segments) <= len(pathTokens) {
			continue
		}
		if prefixMatches(command.Segments, pathTokens) {
			return true
		}
	}
	return false
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
