package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/chzyer/readline"
	bngpb "github.com/veesix-networks/osvbng/api/proto"
)

type CLI struct {
	client           bngpb.BNGServiceClient
	serverAddr       string
	rl               *readline.Instance
	running          bool
	tree             *CommandTree
	devMode          bool
	dockerComposeDir string
	currentLine      string
	configMode       bool
	configSessionID  string
}

func NewCLI(client bngpb.BNGServiceClient, serverAddr string, devMode bool, dockerComposeDir string) *CLI {
	cli := &CLI{
		client:           client,
		serverAddr:       serverAddr,
		running:          true,
		tree:             NewCommandTree(),
		devMode:          devMode,
		dockerComposeDir: dockerComposeDir,
	}

	RegisterCommands(cli.tree)

	return cli
}

func (c *CLI) ExecVPP(args ...string) (string, error) {
	var cmd *exec.Cmd

	if c.devMode {
		cmdArgs := append([]string{"compose", "exec", "-T", "bng-vpp", "vppctl"}, args...)
		cmd = exec.Command("docker", cmdArgs...)
		cmd.Dir = c.dockerComposeDir
	} else {
		cmd = exec.Command("vppctl", args...)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("vppctl error: %w\nOutput: %s", err, string(output))
	}

	return string(output), nil
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
			} else if err == io.EOF {
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

func (c *CLI) printBanner() {
	fmt.Println("=====================================")
	fmt.Println("    osvbng Interactive CLI")
	fmt.Println("=====================================")
	fmt.Printf("Connected to: %s\n", c.serverAddr)
	fmt.Println("Type 'help' for available commands")
	fmt.Println("Type 'exit' or 'quit' to exit")
	fmt.Println()
}

func (c *CLI) OnChange(line []rune, pos int, key rune) (newLine []rune, newPos int, ok bool) {
	c.currentLine = string(line)
	return nil, 0, false
}

func (c *CLI) filterInputWithHelp(r rune) (rune, bool) {
	if r == '?' {
		fmt.Print("?\n")
		c.showInlineHelp()
		c.rl.Write([]byte(c.currentLine))
		return 0, false
	}
	return filterInput(r)
}

func (c *CLI) showInlineHelp() {
	input := c.currentLine
	if strings.HasSuffix(input, " ") {
		c.tree.ShowHelp(strings.TrimSpace(input), c.devMode)
	} else {
		completions := c.tree.GetCompletions(input, c.devMode)
		if len(completions) > 0 {
			fmt.Println()
			for _, comp := range completions {
				fmt.Printf("  %s\n", comp)
			}
			fmt.Println()
		} else {
			c.tree.ShowHelp(input, c.devMode)
		}
	}
}

func (c *CLI) processCommand(line string) error {
	if line == "exit" || line == "quit" {
		c.running = false
		return nil
	}

	if line == "?" {
		c.tree.ShowHelp("", c.devMode)
		return nil
	}

	if strings.HasSuffix(line, "?") && !strings.HasPrefix(line, "vppctl ") {
		input := strings.TrimSuffix(line, "?")
		if strings.HasSuffix(input, " ") {
			c.tree.ShowHelp(strings.TrimSpace(input), c.devMode)
		} else {
			completions := c.tree.GetCompletions(input, c.devMode)
			if len(completions) > 0 {
				fmt.Println()
				for _, comp := range completions {
					fmt.Printf("  %s\n", comp)
				}
				fmt.Println()
			} else {
				c.tree.ShowHelp(input, c.devMode)
			}
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return c.tree.Execute(ctx, c, line)
}
