package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	bngpb "github.com/veesix-networks/osvbng/api/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	_ "github.com/veesix-networks/osvbng/cmd/osvbngcli/commands/all"
	_ "github.com/veesix-networks/osvbng/plugins/all"
)

var (
	serverAddr       = flag.String("server", "localhost:50050", "BNG gateway address")
	dockerComposeDir = flag.String("compose-dir", "test-infra", "Docker compose directory")
)

var devMode bool

func main() {
	flag.Parse()

	if os.Getenv("BNG_DEV_MODE") == "1" {
		devMode = true
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	conn, err := grpc.NewClient(*serverAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to %s: %v\n", *serverAddr, err)
		fmt.Fprintf(os.Stderr, "Make sure osvbngd is running\n")
		os.Exit(1)
	}
	defer conn.Close()

	client := bngpb.NewBNGServiceClient(conn)

	cli := NewCLI(client, *serverAddr, devMode, *dockerComposeDir)

	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		cli.Stop()
		os.Exit(0)
	}()

	if err := cli.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
