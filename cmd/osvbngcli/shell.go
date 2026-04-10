package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type BuiltinCommand struct {
	Name        string
	Description string
}

type Suggestion struct {
	Text        string
	Description string
}

type Invocation struct {
	PathTokens []string
	FlagTokens []string
	Format     OutputFormat
}

var builtinCommands = []BuiltinCommand{
	{Name: "help", Description: "Show command help"},
	{Name: "configure", Description: "Enter configuration mode"},
	{Name: "commit", Description: "Commit configuration changes"},
	{Name: "discard", Description: "Discard configuration changes"},
	{Name: "exit", Description: "Exit the CLI"},
	{Name: "quit", Description: "Exit the CLI"},
}

func parseInvocation(line string) (*Invocation, error) {
	tokens, err := tokenizeLine(line, false)
	if err != nil {
		return nil, err
	}

	commandTokens, format, err := splitPipeline(tokens)
	if err != nil {
		return nil, err
	}

	pathEnd := len(commandTokens)
	for i, token := range commandTokens {
		if strings.HasPrefix(token, "--") {
			pathEnd = i
			break
		}
	}

	return &Invocation{
		PathTokens: commandTokens[:pathEnd],
		FlagTokens: commandTokens[pathEnd:],
		Format:     format,
	}, nil
}

func splitPipeline(tokens []string) ([]string, OutputFormat, error) {
	format := FormatCLI
	pipeIndex := -1
	for i, token := range tokens {
		if token == "|" {
			pipeIndex = i
			break
		}
	}
	if pipeIndex == -1 {
		return tokens, format, nil
	}

	if pipeIndex+1 >= len(tokens) {
		return nil, "", fmt.Errorf("missing output format after '|'")
	}
	if pipeIndex+2 != len(tokens) {
		return nil, "", fmt.Errorf("only one output pipeline is supported")
	}

	switch OutputFormat(tokens[pipeIndex+1]) {
	case FormatCLI, FormatJSON, FormatYAML:
		format = OutputFormat(tokens[pipeIndex+1])
	default:
		return nil, "", fmt.Errorf("unsupported output format %q", tokens[pipeIndex+1])
	}

	return tokens[:pipeIndex], format, nil
}

func tokenizeLine(input string, allowOpenQuote bool) ([]string, error) {
	var (
		tokens      []string
		current     strings.Builder
		quote       rune
		escaped     bool
		tokenActive bool
	)

	flush := func() {
		if tokenActive {
			tokens = append(tokens, current.String())
			current.Reset()
			tokenActive = false
		}
	}

	for _, r := range input {
		if escaped {
			current.WriteRune(r)
			tokenActive = true
			escaped = false
			continue
		}

		if quote != 0 {
			switch {
			case r == '\\' && quote == '"':
				escaped = true
			case r == quote:
				quote = 0
			default:
				current.WriteRune(r)
				tokenActive = true
			}
			continue
		}

		switch r {
		case '\\':
			escaped = true
			tokenActive = true
		case '\'', '"':
			quote = r
			tokenActive = true
		case ' ', '\t', '\n':
			flush()
		default:
			current.WriteRune(r)
			tokenActive = true
		}
	}

	if escaped && !allowOpenQuote {
		return nil, fmt.Errorf("unterminated escape sequence")
	}
	if quote != 0 && !allowOpenQuote {
		return nil, fmt.Errorf("unterminated quoted string")
	}

	flush()
	return tokens, nil
}

func (contract *Contract) matchCommand(pathTokens []string) (*GeneratedCommand, map[string]string, error) {
	type match struct {
		command      *GeneratedCommand
		params       map[string]string
		literalScore int
	}

	var matches []match
	for _, command := range contract.Commands {
		params, literalScore, ok := command.matchPath(pathTokens)
		if !ok {
			continue
		}
		matches = append(matches, match{
			command:      command,
			params:       params,
			literalScore: literalScore,
		})
	}

	if len(matches) == 0 {
		return nil, nil, fmt.Errorf("unrecognized command")
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].literalScore != matches[j].literalScore {
			return matches[i].literalScore > matches[j].literalScore
		}
		return matches[i].command.Path < matches[j].command.Path
	})

	return matches[0].command, matches[0].params, nil
}

func (contract *Contract) matchCommandForInvocation(pathTokens []string) (*GeneratedCommand, map[string]string, *string, error) {
	command, params, err := contract.matchCommand(pathTokens)
	if err == nil {
		return command, params, nil, nil
	}

	type match struct {
		command      *GeneratedCommand
		params       map[string]string
		literalScore int
		value        string
	}

	var matches []match
	for _, candidate := range contract.Commands {
		if candidate.positionalScalarFlag() == nil {
			continue
		}
		if len(pathTokens) != len(candidate.Segments)+1 {
			continue
		}

		params, literalScore, ok := candidate.matchPath(pathTokens[:len(candidate.Segments)])
		if !ok {
			continue
		}

		matches = append(matches, match{
			command:      candidate,
			params:       params,
			literalScore: literalScore,
			value:        pathTokens[len(pathTokens)-1],
		})
	}

	if len(matches) == 0 {
		return nil, nil, nil, err
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].literalScore != matches[j].literalScore {
			return matches[i].literalScore > matches[j].literalScore
		}
		return matches[i].command.Path < matches[j].command.Path
	})

	return matches[0].command, matches[0].params, &matches[0].value, nil
}

func (command *GeneratedCommand) matchPath(tokens []string) (map[string]string, int, bool) {
	if len(tokens) != len(command.Segments) {
		return nil, 0, false
	}

	params := make(map[string]string)
	literalScore := 0

	for i, segment := range command.Segments {
		if segment.Param != nil {
			params[segment.Param.Name] = tokens[i]
			continue
		}
		if segment.Literal != tokens[i] {
			return nil, 0, false
		}
		literalScore++
	}

	return params, literalScore, true
}

func (command *GeneratedCommand) apiCommandPath(params map[string]string) string {
	parts := make([]string, 0, len(command.Segments)-1)
	for _, segment := range command.Segments[1:] {
		if segment.Param != nil {
			parts = append(parts, url.PathEscape(params[segment.Param.Name]))
			continue
		}
		parts = append(parts, segment.Literal)
	}

	var prefix string
	switch command.Kind {
	case CommandShow:
		prefix = "/api/show/"
	case CommandSet:
		prefix = "/api/set/"
	case CommandExec:
		prefix = "/api/exec/"
	default:
		prefix = "/api/"
	}

	return prefix + strings.Join(parts, "/")
}

func (command *GeneratedCommand) parseFlags(flagTokens []string, positionalValue *string) (url.Values, interface{}, error) {
	query := url.Values{}
	scalarFlag := command.positionalScalarFlag()
	if len(flagTokens) == 0 && positionalValue == nil {
		for _, flag := range command.requiredFlags() {
			if scalarFlag == flag {
				return nil, nil, fmt.Errorf("missing value")
			}
			return nil, nil, fmt.Errorf("missing required flag --%s", flag.CLIName)
		}
		return query, nil, nil
	}

	flagMap := make(map[string]*FlagSpec)
	for _, flag := range command.allFlags() {
		flagMap["--"+flag.CLIName] = flag
	}

	provided := make(map[string]int)
	bodyValues := make(map[string]interface{})
	var topLevelScalar []interface{}
	var rawJSON interface{}

	for i := 0; i < len(flagTokens); i++ {
		token := flagTokens[i]
		if !strings.HasPrefix(token, "--") {
			return nil, nil, fmt.Errorf("unexpected token %q", token)
		}

		flag := flagMap[token]
		if flag == nil {
			return nil, nil, fmt.Errorf("unknown flag %q", token)
		}

		valueToken, consumed, err := readFlagValue(flag, flagTokens, i)
		if err != nil {
			return nil, nil, err
		}
		i += consumed

		provided[flag.CLIName]++
		if !flag.Repeated && provided[flag.CLIName] > 1 {
			return nil, nil, fmt.Errorf("flag --%s may only be specified once", flag.CLIName)
		}

		switch flag.Location {
		case FlagQuery:
			query.Add(flag.SourceName, valueToken)
		case FlagBody:
			if command.Body.Mode == BodyModeRawJSON {
				if err := json.Unmarshal([]byte(valueToken), &rawJSON); err != nil {
					return nil, nil, fmt.Errorf("invalid JSON for --json: %w", err)
				}
				continue
			}

			parsed, err := parseScalarValue(flag.Kind, valueToken)
			if err != nil {
				return nil, nil, fmt.Errorf("parse --%s: %w", flag.CLIName, err)
			}

			if flag.TopLevelBody {
				if flag.Repeated {
					topLevelScalar = append(topLevelScalar, parsed)
				} else {
					topLevelScalar = []interface{}{parsed}
				}
				continue
			}

			assignBodyValue(bodyValues, flag.BodyPath, parsed, flag.Repeated)
		}
	}

	for _, flag := range command.requiredFlags() {
		if positionalValue != nil && scalarFlag == flag {
			continue
		}
		if provided[flag.CLIName] == 0 {
			if scalarFlag == flag {
				return nil, nil, fmt.Errorf("missing value")
			}
			return nil, nil, fmt.Errorf("missing required flag --%s", flag.CLIName)
		}
	}

	switch command.Body.Mode {
	case BodyModeNone:
		return query, nil, nil
	case BodyModeRawJSON:
		return query, rawJSON, nil
	case BodyModeScalar:
		if positionalValue != nil {
			if scalarFlag == nil {
				return nil, nil, fmt.Errorf("unexpected positional value %q", *positionalValue)
			}
			if provided[scalarFlag.CLIName] > 0 {
				return nil, nil, fmt.Errorf("value provided both positionally and via --%s", scalarFlag.CLIName)
			}
			parsed, err := parseScalarValue(scalarFlag.Kind, *positionalValue)
			if err != nil {
				return nil, nil, fmt.Errorf("parse positional value: %w", err)
			}
			if scalarFlag.Repeated {
				return query, []interface{}{parsed}, nil
			}
			return query, parsed, nil
		}
		if len(topLevelScalar) == 0 {
			return query, nil, nil
		}
		if command.Body.Flags[0].Repeated {
			return query, topLevelScalar, nil
		}
		return query, topLevelScalar[0], nil
	case BodyModeFlattened:
		if len(bodyValues) == 0 {
			return query, nil, nil
		}
		return query, bodyValues, nil
	default:
		return query, nil, nil
	}
}

func (command *GeneratedCommand) positionalScalarFlag() *FlagSpec {
	if command.Body.Mode != BodyModeScalar || len(command.Body.Flags) != 1 {
		return nil
	}
	flag := command.Body.Flags[0]
	if !flag.TopLevelBody || flag.CLIName != "value" || flag.Repeated {
		return nil
	}
	return flag
}

func (command *GeneratedCommand) allFlags() []*FlagSpec {
	flags := make([]*FlagSpec, 0, len(command.QueryFlags)+len(command.Body.Flags))
	flags = append(flags, command.QueryFlags...)
	flags = append(flags, command.Body.Flags...)
	return flags
}

func (command *GeneratedCommand) requiredFlags() []*FlagSpec {
	var required []*FlagSpec
	for _, flag := range command.allFlags() {
		if flag.Required {
			required = append(required, flag)
		}
	}
	return required
}

func readFlagValue(flag *FlagSpec, tokens []string, index int) (string, int, error) {
	if index+1 >= len(tokens) {
		if flag.Kind == ValueBoolean {
			return "true", 0, nil
		}
		return "", 0, fmt.Errorf("flag --%s requires a value", flag.CLIName)
	}

	next := tokens[index+1]
	if strings.HasPrefix(next, "--") || next == "|" {
		if flag.Kind == ValueBoolean {
			return "true", 0, nil
		}
		return "", 0, fmt.Errorf("flag --%s requires a value", flag.CLIName)
	}

	return next, 1, nil
}

func parseScalarValue(kind ValueKind, value string) (interface{}, error) {
	switch kind {
	case ValueBoolean:
		return strconv.ParseBool(value)
	case ValueInteger:
		return strconv.ParseInt(value, 10, 64)
	case ValueNumber:
		return strconv.ParseFloat(value, 64)
	default:
		return value, nil
	}
}

func assignBodyValue(root map[string]interface{}, path []string, value interface{}, repeated bool) {
	if len(path) == 0 {
		return
	}

	current := root
	for i := 0; i < len(path)-1; i++ {
		key := path[i]
		next, ok := current[key]
		if !ok {
			child := make(map[string]interface{})
			current[key] = child
			current = child
			continue
		}

		child, ok := next.(map[string]interface{})
		if !ok {
			child = make(map[string]interface{})
			current[key] = child
		}
		current = child
	}

	last := path[len(path)-1]
	if !repeated {
		current[last] = value
		return
	}

	existing, ok := current[last]
	if !ok {
		current[last] = []interface{}{value}
		return
	}

	values, ok := existing.([]interface{})
	if !ok {
		values = []interface{}{existing}
	}
	current[last] = append(values, value)
}

func formatPlaceholder(name string) string {
	return "<" + strings.ReplaceAll(name, "_", "-") + ">"
}

func uniqueSuggestions(items []Suggestion) []Suggestion {
	seen := make(map[string]bool, len(items))
	result := make([]Suggestion, 0, len(items))
	for _, item := range items {
		if item.Text == "" || seen[item.Text] {
			continue
		}
		seen[item.Text] = true
		result = append(result, item)
	}
	return result
}
