package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/veesix-networks/osvbng/pkg/version"
)

var (
	serverAddr  = flag.String("server", "auto", "Northbound API base URL; \"auto\" probes /run/osvbng/api.sock then falls back to http://localhost:8080")
	showVersion = flag.Bool("version", false, "Print version and exit")
)

func main() {
	flag.Parse()

	if *showVersion {
		fmt.Println(version.Full())
		return
	}

	cli, err := NewCLI(*serverAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigCh
		// Mode-aware path: if an upgrade is in flight, route the
		// first signal into a context cancel so the upgrade can run
		// its own cleanup (drop-in removal, daemon restart, journal
		// finalisation) rather than os.Exit(0) skipping every defer.
		// A second signal during the cleanup window forces hard exit.
		if interruptActiveUpgrade() {
			fmt.Println("\nUpgrade interrupt requested; finishing cleanup. Send another Ctrl+C to force exit.")
			<-sigCh
			os.Exit(2)
		}
		fmt.Println("\nShutting down...")
		cli.Stop()
		os.Exit(0)
	}()

	if err := cli.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
