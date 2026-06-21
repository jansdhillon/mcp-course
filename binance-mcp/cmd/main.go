package main

import (
	"context"
	"encoding/json"
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

func getPrice(symbol string) (*PriceToolResult, error) {
	url := BINANCE_PRICE_API_URL + "?symbol=" + getSymbolFromName(symbol)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()
	var data PriceToolResult

	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		return nil, err
	}

	return &data, nil
}

type PriceToolParams struct {
	Symbol string `json:"symbol"`
}

type PriceToolResult struct {
	Price  string `json:"price" jsonschema:"the price of the ticker symbol"`
	Symbol string `json:"symbol" jsonschema:"the ticket symbol"`
}

func priceTool(ctx context.Context, req *mcp.CallToolRequest, args PriceToolParams) (*mcp.CallToolResult, *PriceToolResult, error) {
	price, err := getPrice(args.Symbol)
	if err != nil {
		return nil, nil, err
	}

	priceJSON, err := json.Marshal(price)
	if err != nil {
		return nil, nil, err
	}

	return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: string(priceJSON),
				},
			},
		}, &PriceToolResult{
			Price:  price.Price,
			Symbol: price.Symbol,
		}, nil
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
	}, priceTool)

	go startServer(ctx, s, errChan)

	log.Printf("MCP server started...")

	if err := <-errChan; err != nil {
		log.Fatalf("failed to start server: %+v", err)
	}

}
