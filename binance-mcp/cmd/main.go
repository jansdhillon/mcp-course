package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	ACTIVITY_LOG_FILE = "activity.log"
	SYMBOL_MAP        = "symbol_map.csv"
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

// readLocalFile reads a local file in the same directory
// as the executable
func readLocalFile(file string) ([]byte, error) {
	execPath, err := os.Executable()
	if err != nil {
		log.Fatalf("failed to find executable path: %v", err)
		return nil, err
	}

	log.Printf("execPath: %s", execPath)

	execDir := filepath.Dir(execPath)

	activityLogFilePath := execDir + "/" + file

	existing, err := os.ReadFile(activityLogFilePath)
	if err != nil {
		log.Fatalf("failed to find read file: %v", err)
		return nil, err
	}

	return existing, nil

}

func writeToLocalFile(file, format string, args ...any) error {
	existing, err := readLocalFile(file)
	if err != nil {
		return err
	}

	logFile, err := os.OpenFile(file, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0777)
	if err != nil {
		return err
	}

	defer logFile.Close()

	_, err = logFile.Write(fmt.Appendf(existing, format, args...))
	if err != nil {
		return err
	}

	return nil

}

func getPriceChangesTool(ctx context.Context, req *mcp.CallToolRequest, params GetPriceChangesToolParams) (*mcp.CallToolResult, *GetPriceChangesToolResult, error) {
	if err := req.Session.Log(ctx, &mcp.LoggingMessageParams{
		Data:  "Calling price changes tool",
		Level: "info",
	}); err != nil {
		return nil, nil, fmt.Errorf("log failed")
	}

	res, err := getPriceChanges(params)
	if err != nil {
		req.Session.Log(ctx, &mcp.LoggingMessageParams{
			Data:  fmt.Sprintf("error getting price changes: %s", err),
			Level: "error",
		})

		writeToLocalFile(ACTIVITY_LOG_FILE, "Error getting price changes for symbol %s: %s\n", params.Symbol, err)
		req.Session.Log(ctx, &mcp.LoggingMessageParams{
			Data:  fmt.Sprintf("failed to open logfile: %s", err),
			Level: "error",
		})
		return nil, nil, fmt.Errorf("failed to get price changes : %s", err)
	}

	writeToLocalFile(ACTIVITY_LOG_FILE, "Successfully got price changes for symbol '%s'. Current time is %s.\n", params.Symbol, time.Now().String())

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

func activityLogFilePathResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	u, err := url.Parse(req.Params.URI)
	if err != nil {
		return nil, err
	}

	req.Session.Log(ctx, &mcp.LoggingMessageParams{
		Data:  fmt.Sprintf("reading activity log file at %s", u),
		Level: "info",
	})

	req.Session.Log(ctx, &mcp.LoggingMessageParams{
		Data:  fmt.Sprintf("path: %s", u.Path),
		Level: "info",
	})

	data, err := os.ReadFile(u.Path)
	if err != nil {
		return nil, err
	}

	req.Session.Log(ctx, &mcp.LoggingMessageParams{
		Data:  fmt.Sprintf("data: %s", data),
		Level: "info",
	})

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				Blob:     data,
				Text:     string(data),
				URI:      req.Params.URI,
				MIMEType: "text/plain",
			},
		},
	}, nil
}

func symbolMapResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	u, err := url.Parse(req.Params.URI)
	if err != nil {
		req.Session.Log(ctx, &mcp.LoggingMessageParams{
			Data:  fmt.Sprintf("failed to parse request URI: %s", err),
			Level: "error",
		})
		writeToLocalFile(ACTIVITY_LOG_FILE, "failed to parse request URI: %s", err)
		return nil, err
	}

	req.Session.Log(ctx, &mcp.LoggingMessageParams{
		Data:  fmt.Sprintf("resource path: %s", u.Path),
		Level: "info",
	})
	writeToLocalFile(ACTIVITY_LOG_FILE, "resource path: %s", u.Path)

	symbol_map_content, err := readLocalFile(SYMBOL_MAP)
	if err != nil {
		log.Fatalf("failed to read symbol map: %v", err)
	}

	req.Session.Log(ctx, &mcp.LoggingMessageParams{
		Data:  fmt.Sprintf("symbol map: %s", symbol_map_content),
		Level: "info",
	})

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      req.Params.URI,
				MIMEType: "text/csv",
				Text:     string(symbol_map_content),
			},
		},
	}, nil

}

func priceResourceTemplate(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	req.Session.Log(ctx, &mcp.LoggingMessageParams{
		Data:  fmt.Sprintf("params URI: %v", req.Params.URI),
		Level: "info",
	})

	u, err := url.Parse(req.Params.URI)
	if err != nil {
		req.Session.Log(ctx, &mcp.LoggingMessageParams{
			Data:  fmt.Sprintf("failed to parse params URI: %v", err),
			Level: "error",
		})
	}

	req.Session.Log(ctx, &mcp.LoggingMessageParams{
		Data:  fmt.Sprintf("path: %s", u.Path),
		Level: "info",
	})

	symbol := strings.TrimPrefix(u.Path, "/~")

	req.Session.Log(ctx, &mcp.LoggingMessageParams{
		Data:  fmt.Sprintf("symbol: %s", symbol),
		Level: "info",
	})

	price, err := getPrice(GetPriceToolParams{
		Symbol: symbol,
	})

	if err != nil {
		req.Session.Log(ctx, &mcp.LoggingMessageParams{
			Data:  fmt.Sprintf("failed to get price: %s", err),
			Level: "error",
		})
		writeToLocalFile(ACTIVITY_LOG_FILE, "failed to get price for symbol '%s'", price)
		return nil, err
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      req.Params.URI,
				MIMEType: "text/plain",
				Text:     price.Price,
			},
		},
	}, nil
}

func priceChangesResourceTemplate(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	req.Session.Log(ctx, &mcp.LoggingMessageParams{
		Data:  fmt.Sprintf("params URI: %v", req.Params.URI),
		Level: "info",
	})

	u, err := url.Parse(req.Params.URI)
	if err != nil {
		req.Session.Log(ctx, &mcp.LoggingMessageParams{
			Data:  fmt.Sprintf("failed to parse params URI: %v", err),
			Level: "error",
		})
	}

	req.Session.Log(ctx, &mcp.LoggingMessageParams{
		Data:  fmt.Sprintf("path: %s", u.Path),
		Level: "info",
	})

	symbol := strings.TrimPrefix(u.Path, "/~")

	req.Session.Log(ctx, &mcp.LoggingMessageParams{
		Data:  fmt.Sprintf("symbol: %s", symbol),
		Level: "info",
	})

	changes, err := getPriceChanges(GetPriceToolParams{
		Symbol: symbol,
	})

	priceChangesJSON, err := json.Marshal(changes)
	if err != nil {
		req.Session.Log(ctx, &mcp.LoggingMessageParams{
			Data:  fmt.Sprintf("failed to get price: %s", err),
			Level: "error",
		})
		writeToLocalFile(ACTIVITY_LOG_FILE, "failed to get price changes for symbol '%s'", symbol)

		return nil, err
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      req.Params.URI,
				MIMEType: "text/plain",
				Text:     string(priceChangesJSON),
			},
		},
	}, nil
}

func executiveSummaryPrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return &mcp.GetPromptResult{
		Description: "Returns an executive summary of Bitcoin and Ethereum",
		Messages: []*mcp.PromptMessage{
			{
				Role: "user",
				Content: &mcp.TextContent{Text: `Get the prices of the following crypto asset: btc, eth
Provide me with an executive summary including the two-sentence summary of the crypto asset, the current price, the price change in the last 24 hours, and the percentage change in the last 24 hours.
When using the get_price and get_price_price_change tools, use the symbol as the argument.
Symbols: For bitcoin/btc, the symbol is "BTCUSDT".
Symbols: For ethereum/eth, the symbol is "ETHUSDT".`},
			},
		},
	}, nil
}

func cryptoSummaryPrompt(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	args := req.Params.Arguments["crypto"]
	req.Session.Log(ctx, &mcp.LoggingMessageParams{
		Data:  fmt.Sprintf("args: %v", args),
		Level: "info",
	})
	return &mcp.GetPromptResult{
		Description: "Returns a summary of a crypto asset",
		Messages: []*mcp.PromptMessage{
			{
				Role: "user",
				Content: &mcp.TextContent{
					Text: fmt.Sprintf(`Get the current price of the following crypto asset:
%s
and also provide a summary of the price changes in the last 24 hours.
When using the get_price and get_price_price_change tools, use the symbol as the argument.
Symbols: For bitcoin/btc, the symbol is "BTCUSDT".
Symbols: For ethereum/eth, the symbol is "ETHUSDT".`, args),
				},
			},
		},
	}, nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL)
	defer stop()

	errChan := make(chan (error))
	defer close(errChan)

	execPath, err := os.Executable()
	if err != nil {
		log.Fatalf("failed to find executable path: %v", err)
	}

	log.Printf("execPath: %s", execPath)

	execDir := filepath.Dir(execPath)

	log.Printf("exec dir: %s", execDir)

	activityLogFilePath := execDir + "/" + ACTIVITY_LOG_FILE
	if _, err := os.Stat(activityLogFilePath); err != nil && strings.Contains(err.Error(), "no such file or directory") {
		log.Printf("creating activity log file at '%s'", activityLogFilePath)
		os.Create(activityLogFilePath)
		os.Chmod(activityLogFilePath, 0777)
	} else {
		log.Printf("using existing log file at '%s'", activityLogFilePath)
	}

	symbolMapPath := execDir + "/" + SYMBOL_MAP
	if _, err := os.Stat(activityLogFilePath); err != nil {
		log.Fatalf("symbol map not found: %s", err)
	}

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

	s.AddResource(&mcp.Resource{
		Name:     "activity-log",
		MIMEType: "text/plain",
		URI:      fmt.Sprintf("file://%s", activityLogFilePath),
	}, activityLogFilePathResource)

	s.AddResourceTemplate(&mcp.ResourceTemplate{
		Name:        "get-price-resource-template",
		MIMEType:    "text/plain",
		URITemplate: "resource://crypto_price/~{symbol}",
	}, priceResourceTemplate)

	s.AddResourceTemplate(&mcp.ResourceTemplate{
		Name:        "get-price-changes-resource-template",
		MIMEType:    "text/plain",
		URITemplate: "resource://crypto_price_changes/~{symbol}",
	}, priceChangesResourceTemplate)
	s.AddResource(
		&mcp.Resource{
			Name:     "symbol-map",
			MIMEType: "text/csv",
			URI:      fmt.Sprintf("file://%s", symbolMapPath),
		}, symbolMapResource)

	s.AddPrompt(&mcp.Prompt{Name: "executive-summary"}, executiveSummaryPrompt)
	s.AddPrompt(&mcp.Prompt{Name: "crypto-summary", Arguments: []*mcp.PromptArgument{
		{
			Name:        "crypto",
			Description: "Name of the crypto asset to get a summary of",
			Required:    true,
		},
	}}, cryptoSummaryPrompt)

	go startServer(ctx, s, errChan)

	log.Printf("MCP server started...")

	if err := <-errChan; err != nil {
		log.Fatalf("failed to start server: %+v", err)
	}

}
