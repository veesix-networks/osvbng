package main

import (
	"github.com/chzyer/readline"
)

func (c *CLI) buildCompleter() readline.AutoCompleter {
	return &treeCompleter{tree: c.tree, cli: c}
}

type treeCompleter struct {
	tree *CommandTree
	cli  *CLI
}

func (tc *treeCompleter) Do(line []rune, pos int) (newLine [][]rune, length int) {
	input := string(line[:pos])
	completions := tc.tree.GetCompletions(input, tc.cli.devMode)

	if len(completions) == 0 {
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

	result := make([][]rune, len(completions))
	for i, c := range completions {
		if len(partialWord) > 0 && len(c) >= len(partialWord) {
			suffix := c[len(partialWord):]
			result[i] = []rune(suffix)
		} else {
			result[i] = []rune(c)
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
