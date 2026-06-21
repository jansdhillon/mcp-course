package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	BINANCE_PRICE_API_URL = "https://api.binance.com/api/v3/ticker/price"
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

func getSymbolFromName(name string) string {
	if slices.Contains([]string{"bitcoin", "btc"}, strings.ToLower(name)) {
		return "BTCUSDT"
	} else if slices.Contains([]string{"ethereum", "eth"}, strings.ToLower(name)) {
		return "ETHUSDT"
	} else {
		return strings.ToUpper(name)
	}

}

func getPrice(symbol string) (map[string]any, error) {
	url := BINANCE_PRICE_API_URL + "?symbol=" + getSymbolFromName(symbol)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest(http.MethodGet, url)
	if err != nil {
		return nil, err
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()
	var data map[string]any

	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		return nil, err
	}

	return data, nil
}

type GetPriceParams struct {
	Symbol string `json:"symbol"`
}

func GetPrice(ctx context.Context, req *mcp.CallToolResult, args GetPriceParams) (*mcp.CallToolResult, any, error) {
	price, err := getPrice(args.Symbol)
	if err != nil {
		return nil, nil, err
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: fmt.Sprintf("%s", price),
			},
		},
	}, nil, nil
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

	mcp.AddTool(s, &mcp.Tool{
		Name:        "Get price of a Binance ticker symbol",
		Description: "Gets the current price of a ticker symbol from Binance",
	}, GetPrice)

	go startServer(ctx, s, errChan)

	log.Printf("MCP server started...")

	if err := <-errChan; err != nil {
		log.Fatalf("failed to start server: %+v", err)
	}

}
