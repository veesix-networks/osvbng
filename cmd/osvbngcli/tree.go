package main

import (
	"context"
	"fmt"
	"strings"
)

type CommandHandler func(ctx context.Context, cli *CLI, args []string) error

type ArgumentType int

const (
	ArgKeyword ArgumentType = iota
	ArgUserInput
	ArgKeywordWithValue
)

type Argument struct {
	Name        string
	Description string
	Type        ArgumentType
	Values      []string
}

type CommandNode struct {
	Name        string
	Description string
	Handler     CommandHandler
	Children    []*CommandNode
	Arguments   []*Argument
	DevOnly     bool
}

type CommandTree struct {
	root *CommandNode
}

func NewCommandTree() *CommandTree {
	return &CommandTree{
		root: &CommandNode{
			Name:     "root",
			Children: make([]*CommandNode, 0),
		},
	}
}

func (t *CommandTree) AddRoot(path []string, description string) {
	current := t.root

	for _, part := range path {
		found := false
		for _, child := range current.Children {
			if child.Name == part {
				current = child
				found = true
				break
			}
		}

		if !found {
			newNode := &CommandNode{
				Name:        part,
				Description: description,
				Children:    make([]*CommandNode, 0),
			}
			current.Children = append(current.Children, newNode)
			current = newNode
		} else if current.Description == "" {
			current.Description = description
		}
	}
}

func (t *CommandTree) AddCommand(path []string, description string, handler CommandHandler, args ...*Argument) {
	t.addCommand(path, description, handler, false, args...)
}

func (t *CommandTree) AddDevCommand(path []string, description string, handler CommandHandler, args ...*Argument) {
	t.addCommand(path, description, handler, true, args...)
}

func (t *CommandTree) addCommand(path []string, description string, handler CommandHandler, devOnly bool, args ...*Argument) {
	current := t.root

	for i, part := range path {
		found := false
		for _, child := range current.Children {
			if child.Name == part {
				current = child
				found = true
				break
			}
		}

		if !found {
			newNode := &CommandNode{
				Name:     part,
				Children: make([]*CommandNode, 0),
			}

			if i == len(path)-1 {
				newNode.Description = description
				newNode.Handler = handler
				newNode.Arguments = args
				newNode.DevOnly = devOnly
			}

			current.Children = append(current.Children, newNode)
			current = newNode
		}
	}
}

func (t *CommandTree) Execute(ctx context.Context, cli *CLI, input string) error {
	tokens := strings.Fields(input)
	if len(tokens) == 0 {
		return nil
	}

	current := t.root
	var cmdNode *CommandNode
	var argStart int

	for i, token := range tokens {
		found := false
		for _, child := range current.Children {
			if child.Name == token {
				current = child
				found = true
				if child.Handler != nil {
					cmdNode = child
					argStart = i + 1
				}
				break
			}
		}

		if !found {
			if cmdNode != nil {
				args := tokens[argStart:]
				return cmdNode.Handler(ctx, cli, args)
			}
			return fmt.Errorf("unrecognized command")
		}
	}

	if cmdNode != nil && cmdNode.Handler != nil {
		args := tokens[argStart:]

		if err := validateArguments(cmdNode, args); err != nil {
			return err
		}

		return cmdNode.Handler(ctx, cli, args)
	}

	return fmt.Errorf("incomplete command")
}

func validateArguments(cmd *CommandNode, args []string) error {
	if len(cmd.Arguments) == 0 {
		return nil
	}

	requiredCount := 0
	var requiredArgNames []string

	for _, arg := range cmd.Arguments {
		if arg.Type == ArgUserInput {
			requiredCount++
			requiredArgNames = append(requiredArgNames, arg.Name)
		}
	}

	actualCount := 0
	for _, arg := range args {
		if arg == "|" {
			break
		}
		actualCount++
	}

	if actualCount < requiredCount {
		if len(requiredArgNames) == 1 {
			return fmt.Errorf("%s required", requiredArgNames[0])
		}
		return fmt.Errorf("missing required arguments: %s", strings.Join(requiredArgNames, ", "))
	}

	return nil
}

func (t *CommandTree) GetCompletions(input string, devMode bool) []string {
	tokens := strings.Fields(input)
	endsWithSpace := len(input) > 0 && input[len(input)-1] == ' '

	current := t.root
	depth := 0

	for i, token := range tokens {
		if !endsWithSpace && i == len(tokens)-1 {
			break
		}

		found := false
		for _, child := range current.Children {
			if child.Name == token {
				current = child
				found = true
				depth = i + 1
				break
			}
		}

		if !found {
			if current.Handler != nil {
				break
			}
			return nil
		}
	}

	var completions []string

	if !endsWithSpace && len(tokens) > 0 {
		prefix := tokens[len(tokens)-1]
		for _, child := range current.Children {
			if child.DevOnly && !devMode {
				continue
			}
			if strings.HasPrefix(child.Name, prefix) {
				completions = append(completions, child.Name)
			}
		}

		if current.Handler != nil && len(current.Arguments) > 0 {
			argTokens := tokens[depth:]

			if len(argTokens)%2 == 1 {
				usedArgs := make(map[string]bool)
				for i := 0; i < len(argTokens)-1; i += 2 {
					usedArgs[argTokens[i]] = true
				}

				for _, arg := range current.Arguments {
					if (arg.Type == ArgKeyword || arg.Type == ArgKeywordWithValue) && !usedArgs[arg.Name] {
						if strings.HasPrefix(arg.Name, prefix) {
							completions = append(completions, arg.Name)
						}
					}
				}
			}
		}
	} else {
		for _, child := range current.Children {
			if child.DevOnly && !devMode {
				continue
			}
			completions = append(completions, child.Name)
		}

		if current.Handler != nil && len(current.Arguments) > 0 {
			argTokens := tokens[depth:]
			if len(argTokens) > 0 && len(argTokens)%2 == 1 {
				lastToken := argTokens[len(argTokens)-1]
				for _, arg := range current.Arguments {
					if arg.Name == lastToken {
						if arg.Type == ArgKeyword && len(arg.Values) > 0 {
							return arg.Values
						} else if arg.Type == ArgKeywordWithValue {
							return []string{}
						}
					}
				}
			} else {
				usedArgs := make(map[string]bool)
				for i := 0; i < len(argTokens); i += 2 {
					usedArgs[argTokens[i]] = true
				}

				for _, arg := range current.Arguments {
					if (arg.Type == ArgKeyword || arg.Type == ArgKeywordWithValue) && !usedArgs[arg.Name] {
						completions = append(completions, arg.Name)
					}
				}
			}
		}
	}

	return completions
}

func (t *CommandTree) ShowHelp(input string, devMode bool) {
	tokens := strings.Fields(input)
	current := t.root
	cmdDepth := 0

	for i, token := range tokens {
		found := false
		for _, child := range current.Children {
			if child.Name == token {
				current = child
				cmdDepth = i + 1
				found = true
				break
			}
		}
		if !found {
			break
		}
	}

	argTokens := tokens[cmdDepth:]

	if len(argTokens) > 0 && current.Handler != nil {
		lastToken := argTokens[len(argTokens)-1]
		for _, arg := range current.Arguments {
			if arg.Name == lastToken {
				if arg.Type == ArgKeyword && len(arg.Values) > 0 {
					fmt.Println()
					for _, val := range arg.Values {
						fmt.Printf("  %s\n", val)
					}
					fmt.Println()
					return
				} else if arg.Type == ArgUserInput {
					fmt.Printf("\n  <%s>  %s\n\n", arg.Name, arg.Description)
					return
				} else if arg.Type == ArgKeywordWithValue {
					fmt.Printf("\n  <value>  %s\n\n", arg.Description)
					return
				}
			}
		}
	}

	if len(current.Children) > 0 {
		fmt.Println()
		for _, child := range current.Children {
			if child.DevOnly && !devMode {
				continue
			}
			if child.Description != "" {
				fmt.Printf("  %-20s %s\n", child.Name, child.Description)
			} else {
				fmt.Printf("  %s\n", child.Name)
			}
		}
		fmt.Println()
		return
	}

	if current.Handler != nil && len(current.Arguments) > 0 {
		usedArgs := make(map[string]bool)
		for i := 0; i < len(argTokens); i += 2 {
			usedArgs[argTokens[i]] = true
		}

		fmt.Println()
		for _, arg := range current.Arguments {
			if usedArgs[arg.Name] {
				continue
			}
			if arg.Type == ArgKeyword {
				if arg.Description != "" {
					fmt.Printf("  %-20s %s\n", arg.Name, arg.Description)
				} else {
					fmt.Printf("  %s\n", arg.Name)
				}
			} else if arg.Type == ArgUserInput {
				if arg.Description != "" {
					fmt.Printf("  <%s>%s %s\n", arg.Name, strings.Repeat(" ", 19-len(arg.Name)), arg.Description)
				} else {
					fmt.Printf("  <%s>\n", arg.Name)
				}
			} else if arg.Type == ArgKeywordWithValue {
				if arg.Description != "" {
					fmt.Printf("  %-20s %s\n", arg.Name, arg.Description)
				} else {
					fmt.Printf("  %s\n", arg.Name)
				}
			}
		}
		fmt.Println()
		return
	}

	fmt.Println("\n  <cr>")
}
