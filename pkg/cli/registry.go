package cli

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type CommandHandler func(ctx context.Context, cli interface{}, args []string) error

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

type Command struct {
	Path        []string
	Description string
	Handler     CommandHandler
	Arguments   []*Argument
	DevOnly     bool
	Source      string
	IsRoot      bool
}

type RootCommand struct {
	Path        []string
	Description string
	Source      string
}

var (
	registry     = make(map[string]*Command)
	rootRegistry = make(map[string]*RootCommand)
	mu           sync.RWMutex
)

func Register(source string, cmd *Command) {
	mu.Lock()
	defer mu.Unlock()

	key := strings.Join(cmd.Path, ".")
	if existing, exists := registry[key]; exists {
		panic(fmt.Sprintf("duplicate command: %v (existing source: %s, new source: %s)",
			cmd.Path, existing.Source, source))
	}

	cmd.Source = source
	registry[key] = cmd
}

func RegisterRoot(source string, root *RootCommand) {
	mu.Lock()
	defer mu.Unlock()

	key := strings.Join(root.Path, ".")
	if existing, exists := rootRegistry[key]; exists {
		panic(fmt.Sprintf("duplicate root command: %v (existing source: %s, new source: %s)",
			root.Path, existing.Source, source))
	}

	root.Source = source
	rootRegistry[key] = root
}

func GetAll() []*Command {
	mu.RLock()
	defer mu.RUnlock()

	cmds := make([]*Command, 0, len(registry))
	for _, cmd := range registry {
		cmds = append(cmds, cmd)
	}
	return cmds
}

func GetAllRoots() []*RootCommand {
	mu.RLock()
	defer mu.RUnlock()

	roots := make([]*RootCommand, 0, len(rootRegistry))
	for _, root := range rootRegistry {
		roots = append(roots, root)
	}
	return roots
}

func Get(path []string) (*Command, bool) {
	mu.RLock()
	defer mu.RUnlock()

	key := strings.Join(path, ".")
	cmd, exists := registry[key]
	return cmd, exists
}

func GetRoot(path []string) (*RootCommand, bool) {
	mu.RLock()
	defer mu.RUnlock()

	key := strings.Join(path, ".")
	root, exists := rootRegistry[key]
	return root, exists
}
