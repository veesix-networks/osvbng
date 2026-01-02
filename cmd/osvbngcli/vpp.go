package main

import (
	"context"
	"fmt"
)

func VPPCommand(vppArgs ...string) CommandHandler {
	return func(ctx context.Context, cli *CLI, args []string) error {
		cmdArgs := append(vppArgs, args...)
		output, err := cli.ExecVPP(cmdArgs...)
		if err != nil {
			return err
		}
		fmt.Print(output)
		return nil
	}
}
