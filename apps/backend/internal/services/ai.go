package services

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"market-project/backend/internal/config"
	"market-project/backend/internal/domain"
)

type AIClient struct {
	defaultProvider string
	providers       map[string]providerConfig
	providerOrder   []string
	httpClient      *http.Client
}

type providerConfig struct {
	Name    string
	BaseURL string
	APIKey  string
	Model   string
}

type resolvedProvider struct {
	ID     string
	Config providerConfig
	OK     bool
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type chatCompletionStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

type parsedModelResponse struct {
	Answer     string   `json:"answer"`
	Bias       string   `json:"bias"`
	KeyPoints  []string `json:"key_points"`
	RiskPoints []string `json:"risk_points"`
	WatchItems []string `json:"watch_items"`
	Levels     struct {
		Support  domain.CopilotLevel `json:"support"`
		Pressure domain.CopilotLevel `json:"pressure"`
		Risk     domain.CopilotLevel `json:"risk"`
	} `json:"levels"`
	NewsContext struct {
		Used  bool              `json:"used"`
		Count int               `json:"count"`
		Items []json.RawMessage `json:"items"`
		Note  string            `json:"note"`
	} `json:"news_context"`
}

type summaryStats struct {
	StartDate   string
	EndDate     string
	StartClose  float64
	EndClose    float64
	High        float64
	Low         float64
	ChangePct   float64
	AverageVol  float64
	LatestVol   float64
	CandleCount int
	RSI14       float64
	MACD        float64
	Signal      float64
	Histogram   float64
	MA5         float64
	MA10        float64
	MA20        float64
	K           float64
	D           float64
	J           float64
	BOLLMid     float64
	BOLLUpper   float64
	BOLLLower   float64
}

func NewAIClient(cfg config.Config) *AIClient {
	providers := map[string]providerConfig{
		"openai": {
			Name:    "OpenAI",
			BaseURL: cfg.OpenAIBaseURL,
			APIKey:  cfg.OpenAIAPIKey,
			Model:   cfg.OpenAIModel,
		},
		"deepseek": {
			Name:    "DeepSeek",
			BaseURL: cfg.DeepSeekBaseURL,
			APIKey:  cfg.DeepSeekAPIKey,
			Model:   cfg.DeepSeekModel,
		},
	}

	return &AIClient{
		defaultProvider: strings.ToLower(strings.TrimSpace(cfg.DefaultAIProvider)),
		providers:       providers,
		providerOrder:   []string{"deepseek", "openai"},
		httpClient:      &http.Client{Timeout: 25 * time.Second},
	}
}

func (c *AIClient) resolveProvider(ctx context.Context, req domain.CopilotQueryRequest) resolvedProvider {
	providerID := strings.ToLower(strings.TrimSpace(req.Provider))
	userAPIKey := strings.TrimSpace(req.ProviderAPIKey)

	if userAPIKey != "" && (providerID == "" || providerID == "auto") {
		if detectedID, ok := c.detectProviderByKey(ctx, userAPIKey); ok {
			providerID = detectedID
		}
	}

	if providerID == "" || providerID == "auto" {
		providerID = c.defaultProvider
	}

	provider, ok := c.providers[providerID]
	if ok && userAPIKey != "" {
		provider.APIKey = userAPIKey
	}

	return resolvedProvider{ID: providerID, Config: provider, OK: ok}
}

func (c *AIClient) detectProviderByKey(ctx context.Context, apiKey string) (string, bool) {
	for _, providerID := range c.providerOrder {
		provider, ok := c.providers[providerID]
		if !ok || provider.BaseURL == "" {
			continue
		}
		if c.canListModels(ctx, provider.BaseURL, apiKey) {
			return providerID, true
		}
	}
	return "", false
}

func (c *AIClient) canListModels(ctx context.Context, baseURL, apiKey string) bool {
	detectCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	endpoint := strings.TrimRight(baseURL, "/") + "/models"
	req, err := http.NewRequestWithContext(detectCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices
}

func (c *AIClient) Query(ctx context.Context, req domain.CopilotQueryRequest, rows []domain.OHLCRow, newsItems []domain.CopilotNewsItem) (domain.CopilotQueryResponse, error) {
	stats := computeSummary(rows)
	if stats.CandleCount == 0 {
		return domain.CopilotQueryResponse{}, fmt.Errorf("no OHLC rows available")
	}

	resolved := c.resolveProvider(ctx, req)
	provider := resolved.Config
	if !resolved.OK || provider.BaseURL == "" || provider.APIKey == "" || provider.Model == "" {
		return heuristicResponse(req, rows, stats, newsItems), nil
	}

	payload := map[string]any{
		"model":    provider.Model,
		"messages": buildMessages(req, stats, newsItems, true),
		"response_format": map[string]string{
			"type": "json_object",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return domain.CopilotQueryResponse{}, err
	}

	endpoint := strings.TrimRight(provider.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return domain.CopilotQueryResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+provider.APIKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return heuristicResponse(req, rows, stats, newsItems), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return heuristicResponse(req, rows, stats, newsItems), nil
	}

	var completion chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&completion); err != nil {
		return heuristicResponse(req, rows, stats, newsItems), nil
	}

	if len(completion.Choices) == 0 {
		return heuristicResponse(req, rows, stats, newsItems), nil
	}

	content := completion.Choices[0].Message.Content
	if parsed, ok := parseModelResponse(content); ok && parsed.Answer != "" {
		fillStructuredFields(&parsed, rows, stats, newsItems)
		return parsed, nil
	}

	fallback := heuristicResponse(req, rows, stats, newsItems)
	fallback.Answer = strings.TrimSpace(content)
	fillStructuredFields(&fallback, rows, stats, newsItems)
	return fallback, nil
}

func (c *AIClient) QueryStream(ctx context.Context, req domain.CopilotQueryRequest, rows []domain.OHLCRow, newsItems []domain.CopilotNewsItem, onDelta func(string)) (domain.CopilotQueryResponse, error) {
	stats := computeSummary(rows)
	if stats.CandleCount == 0 {
		return domain.CopilotQueryResponse{}, fmt.Errorf("no OHLC rows available")
	}

	resolved := c.resolveProvider(ctx, req)
	provider := resolved.Config
	if !resolved.OK || provider.BaseURL == "" || provider.APIKey == "" || provider.Model == "" {
		response := heuristicResponse(req, rows, stats, newsItems)
		if onDelta != nil && response.Answer != "" {
			onDelta(response.Answer)
		}
		return response, nil
	}

	payload := map[string]any{
		"model":    provider.Model,
		"messages": buildMessages(req, stats, newsItems, false),
		"stream":   true,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return domain.CopilotQueryResponse{}, err
	}

	endpoint := strings.TrimRight(provider.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return domain.CopilotQueryResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+provider.APIKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		response := heuristicResponse(req, rows, stats, newsItems)
		if onDelta != nil && response.Answer != "" {
			onDelta(response.Answer)
		}
		return response, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		response := heuristicResponse(req, rows, stats, newsItems)
		if onDelta != nil && response.Answer != "" {
			onDelta(response.Answer)
		}
		return response, nil
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	var answerBuilder strings.Builder
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}

		var chunk chatCompletionStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		for _, choice := range chunk.Choices {
			content := choice.Delta.Content
			if content == "" {
				continue
			}
			answerBuilder.WriteString(content)
			if onDelta != nil {
				onDelta(content)
			}
		}
	}

	answer := strings.TrimSpace(answerBuilder.String())
	if answer == "" {
		response := heuristicResponse(req, rows, stats, newsItems)
		if onDelta != nil && response.Answer != "" {
			onDelta(response.Answer)
		}
		return response, nil
	}

	response := heuristicResponse(req, rows, stats, newsItems)
	response.Answer = answer
	fillStructuredFields(&response, rows, stats, newsItems)
	return response, scanner.Err()
}

func parseModelResponse(content string) (domain.CopilotQueryResponse, bool) {
	cleaned := strings.TrimSpace(content)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var parsed parsedModelResponse
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		return domain.CopilotQueryResponse{}, false
	}

	result := domain.CopilotQueryResponse{
		Answer:     strings.TrimSpace(parsed.Answer),
		Bias:       strings.TrimSpace(parsed.Bias),
		KeyPoints:  parsed.KeyPoints,
		RiskPoints: parsed.RiskPoints,
		WatchItems: parsed.WatchItems,
		Levels: domain.CopilotLevels{
			Support:  parsed.Levels.Support,
			Pressure: parsed.Levels.Pressure,
			Risk:     parsed.Levels.Risk,
		},
		NewsContext: domain.CopilotNewsContext{
			Used:  parsed.NewsContext.Used,
			Count: parsed.NewsContext.Count,
			Note:  strings.TrimSpace(parsed.NewsContext.Note),
		},
	}

	for _, item := range parsed.NewsContext.Items {
		if len(item) == 0 {
			continue
		}

		var objectItem domain.CopilotNewsItem
		if err := json.Unmarshal(item, &objectItem); err == nil && (objectItem.Title != "" || objectItem.URL != "") {
			result.NewsContext.Items = append(result.NewsContext.Items, objectItem)
			continue
		}

		var textItem string
		if err := json.Unmarshal(item, &textItem); err == nil && strings.TrimSpace(textItem) != "" {
			result.NewsContext.Items = append(result.NewsContext.Items, domain.CopilotNewsItem{
				Title: strings.TrimSpace(textItem),
			})
		}
	}

	return result, result.Answer != ""
}

func (c *AIClient) Providers() []domain.AIProviderInfo {
	result := make([]domain.AIProviderInfo, 0, len(c.providers))
	for id, provider := range c.providers {
		enabled := provider.BaseURL != "" && provider.APIKey != "" && provider.Model != ""
		result = append(result, domain.AIProviderInfo{
			ID:        id,
			Name:      provider.Name,
			Model:     provider.Model,
			Enabled:   enabled,
			IsDefault: id == c.defaultProvider,
		})
	}
	return result
}

func buildMessages(req domain.CopilotQueryRequest, stats summaryStats, newsItems []domain.CopilotNewsItem, structured bool) []map[string]string {
	messages := []map[string]string{
		buildSystemMessage(structured),
	}

	for _, item := range req.History {
		role := strings.TrimSpace(strings.ToLower(item.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		messages = append(messages, map[string]string{
			"role":    role,
			"content": content,
		})
	}

	messages = append(messages, map[string]string{
		"role":    "user",
		"content": buildPrompt(req, stats, newsItems),
	})

	return messages
}

func buildSystemMessage(structured bool) map[string]string {
	if structured {
		return map[string]string{
			"role":    "system",
			"content": "You are a market copilot. Respond in JSON with keys answer, bias, key_points, risk_points, watch_items, levels, news_context. Bias must be bullish, bearish, or neutral. levels must contain support, pressure, risk with value and reason. news_context must contain used, count, items, note. Keep answers concise, specific to the chart range, and suitable for Chinese-speaking traders.",
		}
	}

	return map[string]string{
		"role":    "system",
		"content": "You are a market copilot. Reply in concise Chinese prose only. Do not output JSON, markdown code fences, or section headers. Focus on the chart range, trend judgment, and the most important near-term watch items.",
	}
}

func buildPrompt(req domain.CopilotQueryRequest, stats summaryStats, newsItems []domain.CopilotNewsItem) string {
	newsLines := []string{"News Evidence:"}
	if len(newsItems) == 0 {
		newsLines = append(newsLines, "- none")
	} else {
		for _, item := range newsItems {
			newsLines = append(newsLines, fmt.Sprintf("- [%s] %s | %s | %s", item.PublishedAt, item.Source, item.Title, item.URL))
		}
	}
	return fmt.Sprintf(
		"Symbol: %s\nQuestion: %s\nRange: %s to %s\nCandles: %d\nStart Close: %.2f\nEnd Close: %.2f\nHigh: %.2f\nLow: %.2f\nChange %%: %.2f\nAverage Volume: %.2f\nLatest Volume: %.2f\nRSI14: %.2f\nMACD: %.4f\nSignal: %.4f\nHistogram: %.4f\nMA5: %.2f\nMA10: %.2f\nMA20: %.2f\nK: %.2f\nD: %.2f\nJ: %.2f\nBOLL Mid: %.2f\nBOLL Upper: %.2f\nBOLL Lower: %.2f\n%s\nUse price structure plus RSI/MACD/MA/KDJ/BOLL and grounded news evidence to explain trend, momentum, volatility compression or expansion, divergence risk, and what to watch next. Return concise levels and a near-term watchlist. news_context.items must contain only grounded items from the evidence list.",
		req.Symbol,
		req.Question,
		stats.StartDate,
		stats.EndDate,
		stats.CandleCount,
		stats.StartClose,
		stats.EndClose,
		stats.High,
		stats.Low,
		stats.ChangePct,
		stats.AverageVol,
		stats.LatestVol,
		stats.RSI14,
		stats.MACD,
		stats.Signal,
		stats.Histogram,
		stats.MA5,
		stats.MA10,
		stats.MA20,
		stats.K,
		stats.D,
		stats.J,
		stats.BOLLMid,
		stats.BOLLUpper,
		stats.BOLLLower,
		strings.Join(newsLines, "\n"),
	)
}

func computeSummary(rows []domain.OHLCRow) summaryStats {
	if len(rows) == 0 {
		return summaryStats{}
	}

	stats := summaryStats{
		StartDate:   rows[0].Date,
		EndDate:     rows[len(rows)-1].Date,
		StartClose:  rows[0].Close,
		EndClose:    rows[len(rows)-1].Close,
		High:        rows[0].High,
		Low:         rows[0].Low,
		LatestVol:   rows[len(rows)-1].Volume,
		CandleCount: len(rows),
	}

	var totalVol float64
	for _, row := range rows {
		if row.High > stats.High {
			stats.High = row.High
		}
		if row.Low < stats.Low {
			stats.Low = row.Low
		}
		totalVol += row.Volume
	}

	stats.AverageVol = totalVol / float64(len(rows))
	if stats.StartClose != 0 {
		stats.ChangePct = ((stats.EndClose - stats.StartClose) / stats.StartClose) * 100
	}
	stats.RSI14 = computeRSI(rows, 14)
	stats.MACD, stats.Signal, stats.Histogram = computeMACD(rows)
	stats.MA5 = computeMA(rows, 5)
	stats.MA10 = computeMA(rows, 10)
	stats.MA20 = computeMA(rows, 20)
	stats.K, stats.D, stats.J = computeKDJ(rows, 9)
	stats.BOLLMid, stats.BOLLUpper, stats.BOLLLower = computeBOLL(rows, 20, 2)

	return stats
}

func computeMA(rows []domain.OHLCRow, period int) float64 {
	if len(rows) < period {
		return 0
	}

	total := 0.0
	for _, row := range rows[len(rows)-period:] {
		total += row.Close
	}
	return total / float64(period)
}

func computeEMA(values []float64, period int) []float64 {
	result := make([]float64, len(values))
	if len(values) < period {
		return result
	}

	multiplier := 2.0 / float64(period+1)
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += values[i]
	}
	prev := sum / float64(period)
	result[period-1] = prev
	for i := period; i < len(values); i++ {
		prev = (values[i]-prev)*multiplier + prev
		result[i] = prev
	}
	return result
}

func computeRSI(rows []domain.OHLCRow, period int) float64 {
	if len(rows) <= period {
		return 0
	}

	avgGain := 0.0
	avgLoss := 0.0
	for i := 1; i <= period; i++ {
		delta := rows[i].Close - rows[i-1].Close
		if delta > 0 {
			avgGain += delta
		} else {
			avgLoss += -delta
		}
	}
	avgGain /= float64(period)
	avgLoss /= float64(period)

	for i := period + 1; i < len(rows); i++ {
		delta := rows[i].Close - rows[i-1].Close
		gain := 0.0
		loss := 0.0
		if delta > 0 {
			gain = delta
		} else {
			loss = -delta
		}
		avgGain = ((avgGain * float64(period-1)) + gain) / float64(period)
		avgLoss = ((avgLoss * float64(period-1)) + loss) / float64(period)
	}

	if avgLoss == 0 {
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - (100 / (1 + rs))
}

func computeMACD(rows []domain.OHLCRow) (float64, float64, float64) {
	if len(rows) < 26 {
		return 0, 0, 0
	}

	closes := make([]float64, len(rows))
	for i, row := range rows {
		closes[i] = row.Close
	}

	ema12 := computeEMA(closes, 12)
	ema26 := computeEMA(closes, 26)
	macdSeries := make([]float64, len(closes))
	for i := range closes {
		macdSeries[i] = ema12[i] - ema26[i]
	}

	signal := computeEMA(macdSeries, 9)
	last := len(closes) - 1
	macd := macdSeries[last]
	sig := signal[last]
	return macd, sig, macd - sig
}

func computeKDJ(rows []domain.OHLCRow, period int) (float64, float64, float64) {
	if len(rows) < period {
		return 0, 0, 0
	}

	k := 50.0
	d := 50.0
	for i := period - 1; i < len(rows); i++ {
		low := rows[i].Low
		high := rows[i].High
		for _, row := range rows[i-period+1 : i+1] {
			if row.Low < low {
				low = row.Low
			}
			if row.High > high {
				high = row.High
			}
		}

		rsv := 50.0
		if high != low {
			rsv = ((rows[i].Close - low) / (high - low)) * 100
		}

		k = (2.0/3.0)*k + (1.0/3.0)*rsv
		d = (2.0/3.0)*d + (1.0/3.0)*k
	}

	j := 3*k - 2*d
	return k, d, j
}

func computeBOLL(rows []domain.OHLCRow, period int, multiplier float64) (float64, float64, float64) {
	if len(rows) < period {
		return 0, 0, 0
	}

	window := rows[len(rows)-period:]
	middle := computeMA(window, len(window))
	variance := 0.0
	for _, row := range window {
		variance += (row.Close - middle) * (row.Close - middle)
	}
	variance /= float64(period)
	deviation := variance
	if deviation > 0 {
		deviation = sqrt(deviation)
	}

	return middle, middle + multiplier*deviation, middle - multiplier*deviation
}

func sqrt(value float64) float64 {
	if value <= 0 {
		return 0
	}

	x := value
	for i := 0; i < 8; i++ {
		x = 0.5 * (x + value/x)
	}
	return x
}

func heuristicResponse(req domain.CopilotQueryRequest, rows []domain.OHLCRow, stats summaryStats, newsItems []domain.CopilotNewsItem) domain.CopilotQueryResponse {
	bias := "neutral"
	if stats.ChangePct > 3 {
		bias = "bullish"
	}
	if stats.ChangePct < -3 {
		bias = "bearish"
	}

	answer := fmt.Sprintf(
		"%s 在 %s 到 %s 期间累计变动 %.2f%%，区间高低点分别为 %.2f 和 %.2f。当前更适合把它看成 %s 场景，先观察价格是否继续沿着最近方向延续，以及成交量能否配合。",
		req.Symbol,
		stats.StartDate,
		stats.EndDate,
		stats.ChangePct,
		stats.High,
		stats.Low,
		bias,
	)

	response := domain.CopilotQueryResponse{
		Answer: answer,
		Bias:   bias,
		KeyPoints: []string{
			fmt.Sprintf("区间收盘涨跌幅为 %.2f%%", stats.ChangePct),
			fmt.Sprintf("最新成交量 %.2f，对比区间均量 %.2f", stats.LatestVol, stats.AverageVol),
			fmt.Sprintf("价格主要活动区间在 %.2f 到 %.2f", stats.Low, stats.High),
			fmt.Sprintf("RSI14 为 %.2f，MACD %.4f / Signal %.4f", stats.RSI14, stats.MACD, stats.Signal),
			fmt.Sprintf("均线结构 MA5 %.2f / MA10 %.2f / MA20 %.2f", stats.MA5, stats.MA10, stats.MA20),
			fmt.Sprintf("KDJ 为 K %.2f / D %.2f / J %.2f", stats.K, stats.D, stats.J),
			fmt.Sprintf("BOLL 为中轨 %.2f，上轨 %.2f，下轨 %.2f", stats.BOLLMid, stats.BOLLUpper, stats.BOLLLower),
		},
		RiskPoints: []string{
			"当前预判只是基于价格结构和区间统计，不是量化预测模型",
			"如果价格跌破区间关键低点，当前判断需要重新评估",
			"如果成交量无法持续配合，趋势延续性会下降",
			"如果 RSI 与 MACD 出现背离，单纯看价格方向会失真",
			"如果短均线重新跌破中长期均线，趋势判断容易反转",
			"如果布林带快速张口后价格回落，短线波动会明显放大",
		},
		WatchItems: []string{
			"观察是否突破区间高点并站稳",
			"观察回撤时是否守住中枢区域",
			"观察放量还是缩量来确认方向可信度",
			"观察 MACD 柱体是否继续扩张，以及 RSI 是否进入超买超卖区",
			"观察 MA5/MA10 是否继续压在 MA20 上方，以及 KDJ 是否继续金叉",
			"观察价格是在布林上轨附近延续强势，还是回到中轨附近重新选择方向",
		},
	}
	fillStructuredFields(&response, rows, stats, newsItems)
	return response
}

func fillStructuredFields(response *domain.CopilotQueryResponse, rows []domain.OHLCRow, stats summaryStats, newsItems []domain.CopilotNewsItem) {
	if response.Levels.Support.Value == 0 && response.Levels.Pressure.Value == 0 && response.Levels.Risk.Value == 0 {
		response.Levels = computeLevels(rows, stats)
	}
	if response.NewsContext.Items == nil {
		response.NewsContext.Items = newsItems
	}
	if response.NewsContext.Count == 0 {
		response.NewsContext.Count = len(response.NewsContext.Items)
	}
	if !response.NewsContext.Used && response.NewsContext.Count > 0 {
		response.NewsContext.Used = true
	}
	if response.NewsContext.Note == "" {
		if response.NewsContext.Count > 0 {
			response.NewsContext.Note = "已补充新闻证据，结论应优先结合图表与新闻共同理解。"
		} else {
			response.NewsContext.Note = "当前请求没有命中可用新闻证据；本轮按纯图表分析处理。"
		}
	}
}

func computeLevels(rows []domain.OHLCRow, stats summaryStats) domain.CopilotLevels {
	recentLow := stats.Low
	recentHigh := stats.High
	if len(rows) > 0 {
		slice := rows
		if len(rows) > 20 {
			slice = rows[len(rows)-20:]
		}
		recentLow = slice[0].Low
		recentHigh = slice[0].High
		for _, row := range slice {
			if row.Low < recentLow {
				recentLow = row.Low
			}
			if row.High > recentHigh {
				recentHigh = row.High
			}
		}
	}

	support := pickSupport([]float64{recentLow, stats.MA10, stats.MA20, stats.BOLLLower}, stats.EndClose)
	pressure := pickPressure([]float64{recentHigh, stats.BOLLUpper, stats.MA10, stats.MA20}, stats.EndClose)
	risk := minFloat(support, recentLow)
	if stats.BOLLLower > 0 {
		risk = minFloat(risk, stats.BOLLLower)
	}
	risk *= 0.985

	return domain.CopilotLevels{
		Support: domain.CopilotLevel{
			Value:  support,
			Reason: fmt.Sprintf("结合 MA10 %.2f / MA20 %.2f / BOLL 下轨 %.2f / 近20根低点 %.2f", stats.MA10, stats.MA20, stats.BOLLLower, recentLow),
		},
		Pressure: domain.CopilotLevel{
			Value:  pressure,
			Reason: fmt.Sprintf("结合 BOLL 上轨 %.2f / 近20根高点 %.2f", stats.BOLLUpper, recentHigh),
		},
		Risk: domain.CopilotLevel{
			Value:  risk,
			Reason: "以更保守的破位止损视角，取布林下轨与近端低点的更弱一侧再下移 1.5%",
		},
	}
}

func pickSupport(candidates []float64, close float64) float64 {
	best := 0.0
	for _, item := range candidates {
		if item <= 0 {
			continue
		}
		if item <= close*1.02 && item > best {
			best = item
		}
	}
	if best == 0 {
		return close
	}
	return best
}

func pickPressure(candidates []float64, close float64) float64 {
	best := 0.0
	for _, item := range candidates {
		if item <= 0 {
			continue
		}
		if item >= close*0.98 && (best == 0 || item < best) {
			best = item
		}
	}
	if best == 0 {
		return close
	}
	return best
}

func minFloat(values ...float64) float64 {
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}
