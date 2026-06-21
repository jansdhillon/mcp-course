package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func startServer(ctx context.Context, s *mcp.Server, errChan chan<- error) {
	for {
		select {
		case <-ctx.Done():
			errChan <- ctx.Err()
		default:
			errChan <- s.Run(ctx, &mcp.StdioTransport{})
		}

	}

}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL)
	defer stop()

	errChan := make(chan (error))
	defer close(errChan)

	s := mcp.NewServer(&mcp.Implementation{
		Name:    "Binance MCP",
		Version: "v1.0.0",
	}, nil)

	go startServer(ctx, s, errChan)

	log.Printf("MCP server started...")

	if err := <-errChan; err != nil {
		log.Fatalf("failed to start server: %+v", err)
	}

}
