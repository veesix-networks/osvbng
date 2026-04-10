package main

import "github.com/chzyer/readline"

func (c *CLI) buildCompleter() readline.AutoCompleter {
	return &cliCompleter{cli: c}
}

type cliCompleter struct {
	cli *CLI
}

func (cc *cliCompleter) Do(line []rune, pos int) (newLine [][]rune, length int) {
	input := string(line[:pos])
	suggestions := cc.cli.suggestionsForInput(input, false)
	if len(suggestions) == 0 {
		return nil, 0
	}

	texts := make([]string, 0, len(suggestions))
	for _, suggestion := range suggestions {
		if suggestion.Text == "" || suggestion.Text == "|" {
			continue
		}
		texts = append(texts, suggestion.Text)
	}
	if len(texts) == 0 {
		return nil, 0
	}

	lastSpace := -1
	for i := pos - 1; i >= 0; i-- {
		if line[i] == ' ' {
			lastSpace = i
			break
		}
	}

	partialWord := ""
	if lastSpace >= 0 {
		partialWord = string(line[lastSpace+1 : pos])
	} else {
		partialWord = string(line[:pos])
	}

	result := make([][]rune, len(texts))
	for i, suggestion := range texts {
		if len(partialWord) > 0 && len(suggestion) >= len(partialWord) {
			result[i] = []rune(suggestion[len(partialWord):])
		} else {
			result[i] = []rune(suggestion)
		}
	}

	return result, len(partialWord)
}

func filterInput(r rune) (rune, bool) {
	switch r {
	case readline.CharCtrlZ:
		return r, false
	}
	return r, true
}

func getPrompt(configMode bool) string {
	if configMode {
		return "bng(config)# "
	}
	return "bng> "
}
