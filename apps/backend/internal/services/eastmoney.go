package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"market-project/backend/internal/domain"
)

type EastmoneyClient struct {
	httpClient *http.Client
}

type eastmoneyResponse struct {
	Data struct {
		Code   string   `json:"code"`
		Name   string   `json:"name"`
		Klines []string `json:"klines"`
	} `json:"data"`
}

type eastmoneySymbolCatalogResponse struct {
	Data struct {
		Total int `json:"total"`
		Diff  []struct {
			Symbol string `json:"f12"`
			Market int    `json:"f13"`
			Name   string `json:"f14"`
		} `json:"diff"`
	} `json:"data"`
}

func NewEastmoneyClient(timeout time.Duration) *EastmoneyClient {
	return &EastmoneyClient{
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *EastmoneyClient) FetchDailyOHLC(ctx context.Context, rawSymbol string) (domain.Symbol, []domain.OHLCRow, error) {
	symbol, market, secID, err := normalizeSymbol(rawSymbol)
	if err != nil {
		return domain.Symbol{}, nil, err
	}

	params := url.Values{}
	params.Set("secid", secID)
	params.Set("fields1", "f1,f2,f3,f4,f5,f6")
	params.Set("fields2", "f51,f52,f53,f54,f55,f56,f57,f58,f59,f60,f61")
	params.Set("klt", "101")
	params.Set("fqt", "1")
	params.Set("beg", "20220101")
	params.Set("end", "20500101")

	endpoint := "https://push2his.eastmoney.com/api/qt/stock/kline/get?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return domain.Symbol{}, nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 market-copilot")
	req.Header.Set("Referer", "https://quote.eastmoney.com/")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return domain.Symbol{}, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return domain.Symbol{}, nil, fmt.Errorf("eastmoney status %d", resp.StatusCode)
	}

	var payload eastmoneyResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return domain.Symbol{}, nil, err
	}

	if payload.Data.Code == "" || len(payload.Data.Klines) == 0 {
		return domain.Symbol{}, nil, fmt.Errorf("no eastmoney data for %s", rawSymbol)
	}

	rows := make([]domain.OHLCRow, 0, len(payload.Data.Klines))
	for _, line := range payload.Data.Klines {
		parts := strings.Split(line, ",")
		if len(parts) < 9 {
			continue
		}

		openValue, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return domain.Symbol{}, nil, err
		}
		closeValue, err := strconv.ParseFloat(parts[2], 64)
		if err != nil {
			return domain.Symbol{}, nil, err
		}
		highValue, err := strconv.ParseFloat(parts[3], 64)
		if err != nil {
			return domain.Symbol{}, nil, err
		}
		lowValue, err := strconv.ParseFloat(parts[4], 64)
		if err != nil {
			return domain.Symbol{}, nil, err
		}
		volumeValue, err := strconv.ParseFloat(parts[5], 64)
		if err != nil {
			return domain.Symbol{}, nil, err
		}

		row := domain.OHLCRow{
			Symbol: symbol,
			Market: market,
			Date:   parts[0],
			Open:   openValue,
			High:   highValue,
			Low:    lowValue,
			Close:  closeValue,
			Volume: volumeValue,
		}

		if len(parts) > 6 {
			if amountValue, err := strconv.ParseFloat(parts[6], 64); err == nil {
				row.Amount = &amountValue
			}
		}

		if len(parts) > 8 {
			if changeRateValue, err := strconv.ParseFloat(parts[8], 64); err == nil {
				row.ChangeRate = &changeRateValue
			}
		}

		rows = append(rows, row)
	}

	return domain.Symbol{
		Symbol: symbol,
		Name:   payload.Data.Name,
		Market: market,
		Source: "eastmoney",
	}, rows, nil
}

func (c *EastmoneyClient) FetchSymbolCatalog(ctx context.Context) ([]domain.Symbol, error) {
	const requestedPageSize = 1000

	var symbols []domain.Symbol
	total := 0
	for page := 1; ; page++ {
		params := url.Values{}
		params.Set("pn", strconv.Itoa(page))
		params.Set("pz", strconv.Itoa(requestedPageSize))
		params.Set("po", "1")
		params.Set("np", "1")
		params.Set("ut", "bd1d9ddb04089700cf9c27f6f7426281")
		params.Set("fltt", "2")
		params.Set("invt", "2")
		params.Set("fid", "f3")
		params.Set("fs", "m:0+t:6,m:0+t:80,m:1+t:2,m:1+t:23")
		params.Set("fields", "f12,f13,f14")

		endpoint := "https://push2.eastmoney.com/api/qt/clist/get?" + params.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 market-copilot")
		req.Header.Set("Referer", "https://quote.eastmoney.com/")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		var payload eastmoneySymbolCatalogResponse
		err = json.NewDecoder(resp.Body).Decode(&payload)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= http.StatusBadRequest {
			return nil, fmt.Errorf("eastmoney status %d", resp.StatusCode)
		}

		if len(payload.Data.Diff) == 0 {
			break
		}
		if payload.Data.Total > 0 {
			total = payload.Data.Total
		}

		for _, item := range payload.Data.Diff {
			if len(item.Symbol) != 6 || strings.TrimSpace(item.Name) == "" {
				continue
			}

			symbols = append(symbols, domain.Symbol{
				Symbol: item.Symbol,
				Name:   strings.TrimSpace(item.Name),
				Market: mapEastmoneyMarket(item.Market),
				Source: "eastmoney",
			})
		}

		if total > 0 && len(symbols) >= total {
			break
		}
		if page > 200 {
			break
		}
	}

	if len(symbols) == 0 {
		return nil, fmt.Errorf("no eastmoney symbol catalog data")
	}

	return symbols, nil
}

func normalizeSymbol(raw string) (symbol string, market string, secID string, err error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	raw = strings.TrimPrefix(raw, "sh")
	raw = strings.TrimPrefix(raw, "sz")

	if len(raw) != 6 {
		return "", "", "", fmt.Errorf("symbol must be 6 digits")
	}

	if strings.HasPrefix(raw, "6") || strings.HasPrefix(raw, "5") || strings.HasPrefix(raw, "9") {
		return raw, "SH", "1." + raw, nil
	}

	return raw, "SZ", "0." + raw, nil
}

func mapEastmoneyMarket(code int) string {
	switch code {
	case 1:
		return "SH"
	case 0:
		return "SZ"
	default:
		return "CN"
	}
}
