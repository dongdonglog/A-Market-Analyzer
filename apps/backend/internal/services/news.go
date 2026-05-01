package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"market-project/backend/internal/domain"
)

type NewsClient struct {
	httpClient *http.Client
}

type eastmoneyNewsPayload struct {
	Result struct {
		Articles []struct {
			Date      string `json:"date"`
			Title     string `json:"title"`
			Content   string `json:"content"`
			MediaName string `json:"mediaName"`
			URL       string `json:"url"`
		} `json:"cmsArticleWebOld"`
	} `json:"result"`
}

func NewNewsClient(timeout time.Duration) *NewsClient {
	return &NewsClient{
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *NewsClient) SearchSymbolNews(ctx context.Context, keyword, startDate, endDate string, limit int) ([]domain.CopilotNewsItem, error) {
	if limit <= 0 {
		limit = 5
	}

	param := map[string]any{
		"uid":           "",
		"keyword":       keyword,
		"type":          []string{"cmsArticleWebOld"},
		"client":        "web",
		"clientVersion": "curr",
		"clientType":    "web",
		"param": map[string]any{
			"cmsArticleWebOld": map[string]any{
				"searchScope": "default",
				"sort":        "time",
				"pageIndex":   1,
				"pageSize":    limit * 3,
				"preTag":      "",
				"postTag":     "",
			},
		},
	}

	rawParam, err := json.Marshal(param)
	if err != nil {
		return nil, err
	}

	endpoint := "https://search-api-web.eastmoney.com/search/jsonp"
	query := url.Values{}
	query.Set("cb", "callback")
	query.Set("param", string(rawParam))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 market-copilot")
	req.Header.Set("Referer", "https://so.eastmoney.com/")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("eastmoney news status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	text := strings.TrimSpace(string(body))
	text = strings.TrimPrefix(text, "callback(")
	text = strings.TrimSuffix(text, ")")

	var payload eastmoneyNewsPayload
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return nil, err
	}

	items := make([]domain.CopilotNewsItem, 0, limit)
	for _, item := range payload.Result.Articles {
		date := strings.TrimSpace(item.Date)
		if !withinRange(date, startDate, endDate) {
			continue
		}
		items = append(items, domain.CopilotNewsItem{
			Title:           strings.TrimSpace(item.Title),
			Source:          strings.TrimSpace(item.MediaName),
			PublishedAt:     date,
			URL:             strings.TrimSpace(item.URL),
			Summary:         strings.TrimSpace(item.Content),
			RelevanceReason: "与当前股票关键词和区间匹配的资讯结果",
		})
		if len(items) >= limit {
			break
		}
	}

	return items, nil
}

func withinRange(datetime, startDate, endDate string) bool {
	datePart := datetime
	if len(datePart) >= 10 {
		datePart = datePart[:10]
	}
	if startDate != "" && datePart < startDate {
		return false
	}
	if endDate != "" && datePart > endDate {
		return false
	}
	return true
}
