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

type GetPriceToolParams struct {
	Symbol string `json:"symbol"`
}

type GetPriceToolResult struct {
	Price  string `json:"price" jsonschema:"the price of the ticker symbol"`
	Symbol string `json:"symbol" jsonschema:"the ticket symbol"`
}

func getPriceChanges(params GetPriceToolParams) (*GetPriceChangesToolResult, error) {
	url := fmt.Sprintf("https://api.binance.com/api/v3/ticker/24hr?symbol=%s", getSymbolFromName(params.Symbol))

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	res, err := client.Get(url)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	data := new(GetPriceChangesToolResult)

	if err := json.NewDecoder(res.Body).Decode(data); err != nil {
		return nil, err
	}

	return data, nil
}

func getPrice(params GetPriceToolParams) (*GetPriceToolResult, error) {
	url := fmt.Sprintf("https://api.binance.com/api/v3/ticker/price?symbol=%s", getSymbolFromName(params.Symbol))

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
	var data GetPriceToolResult

	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		return nil, err
	}

	return &data, nil
}

func getPriceTool(ctx context.Context, req *mcp.CallToolRequest, params GetPriceToolParams) (*mcp.CallToolResult, *GetPriceToolResult, error) {
	price, err := getPrice(params)
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
	}, price, nil
}

type GetPriceChangesToolParams = GetPriceToolParams

type GetPriceChangesToolResult struct {
	AskPrice           string `json:"askPrice"`
	AskQty             string `json:"askQty"`
	BidPrice           string `json:"bidPrice"`
	BidQty             string `json:"bidQty"`
	CloseTime          int64  `json:"closeTime"`
	Count              int    `json:"count"`
	FirstID            int64  `json:"firstId"`
	HighPrice          string `json:"highPrice"`
	LastID             int64  `json:"lastId"`
	LastPrice          string `json:"lastPrice"`
	LastQty            string `json:"lastQty"`
	LowPrice           string `json:"lowPrice"`
	OpenPrice          string `json:"openPrice"`
	OpenTime           int64  `json:"openTime"`
	PrevClosePrice     string `json:"prevClosePrice"`
	PriceChange        string `json:"priceChange"`
	PriceChangePercent string `json:"priceChangePercent"`
	QuoteVolume        string `json:"quoteVolume"`
	Symbol             string `json:"symbol"`
	Volume             string `json:"volume"`
	WeightedAvgPrice   string `json:"weightedAvgPrice"`
}

func getPriceChangesTool(ctx context.Context, req *mcp.CallToolRequest, params GetPriceChangesToolParams) (*mcp.CallToolResult, *GetPriceChangesToolResult, error) {
	res, err := getPriceChanges(params)
	if err != nil {
		return nil, nil, err
	}

	priceJSON, err := json.Marshal(res)
	if err != nil {
		return nil, nil, err
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: string(priceJSON),
			},
		},
	}, res, nil
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
		Name:        "get-price",
		Description: "Gets the current price of a ticker symbol from Binance",
	}, getPriceTool)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get-price-changes",
		Description: "Gets the price changes in the last 24 hours",
	}, getPriceChangesTool)

	go startServer(ctx, s, errChan)

	log.Printf("MCP server started...")

	if err := <-errChan; err != nil {
		log.Fatalf("failed to start server: %+v", err)
	}

}
