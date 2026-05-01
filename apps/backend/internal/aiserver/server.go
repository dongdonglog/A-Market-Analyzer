package aiserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"market-project/backend/internal/cache"
	"market-project/backend/internal/config"
	"market-project/backend/internal/copilot"
	"market-project/backend/internal/database"
	"market-project/backend/internal/domain"
	"market-project/backend/internal/services"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type RouterDeps struct {
	cfg       config.Config
	cache     *cache.Client
	repo      *database.Repository
	eastmoney *services.EastmoneyClient
	copilot   *copilot.Service
}

func sessionListCacheKey(userID, symbol string) string {
	return fmt.Sprintf("cache:copilot:sessions:%s:%s", userID, symbol)
}

func sessionMessagesCacheKey(userID, sessionID string) string {
	return fmt.Sprintf("cache:copilot:messages:%s:%s", userID, sessionID)
}

func billingSummaryCacheKey(userID string) string {
	return fmt.Sprintf("cache:billing:user:%s", userID)
}

type forwardedAuth struct {
	UserID string
	Email  string
}

func NewRouter(cfg config.Config, repo *database.Repository, eastmoney *services.EastmoneyClient, news *services.NewsClient, ai *services.AIClient, redisClient *cache.Client) *gin.Engine {
	deps := RouterDeps{
		cfg:       cfg,
		cache:     redisClient,
		repo:      repo,
		eastmoney: eastmoney,
		copilot:   copilot.NewService(repo, news, ai),
	}

	router := gin.Default()
	router.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CORSOrigins,
		AllowOriginFunc:  allowOrigin(cfg.CORSOrigins),
		AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowHeaders:     []string{"Authorization", "Content-Type", "X-User-ID", "X-User-Email"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	router.GET("/health", deps.health)

	secured := router.Group("/")
	secured.Use(deps.requireForwardedAuth)
	secured.GET("/ai/providers", deps.listAIProviders)
	secured.GET("/copilot/sessions", deps.listCopilotSessions)
	secured.GET("/copilot/sessions/:id/messages", deps.getCopilotSessionMessages)
	secured.POST("/copilot/sessions/:id/favorite", deps.toggleCopilotSessionFavorite)
	secured.POST("/copilot/query", deps.queryCopilot)
	secured.POST("/copilot/stream", deps.streamCopilot)

	return router
}

func allowOrigin(allowed []string) func(string) bool {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, origin := range allowed {
		allowedSet[origin] = struct{}{}
	}

	return func(origin string) bool {
		if _, ok := allowedSet[origin]; ok {
			return true
		}
		return strings.Contains(origin, "localhost") || strings.Contains(origin, "127.0.0.1")
	}
}

func (d RouterDeps) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"service":   "ai-go",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (d RouterDeps) listAIProviders(c *gin.Context) {
	ctx := c.Request.Context()
	cacheKey := "cache:ai:providers"

	var cached []domain.AIProviderInfo
	if hit, err := d.cache.GetJSON(ctx, cacheKey, &cached); err == nil && hit {
		c.JSON(http.StatusOK, cached)
		return
	}

	providers := d.copilot.Providers()
	_ = d.cache.SetJSON(ctx, cacheKey, providers, d.cfg.AIProvidersTTL)
	c.JSON(http.StatusOK, providers)
}

func (d RouterDeps) queryCopilot(c *gin.Context) {
	ctx := c.Request.Context()
	var req domain.CopilotQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid copilot payload"})
		return
	}
	if strings.TrimSpace(req.Symbol) == "" || strings.TrimSpace(req.Question) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "symbol and question are required"})
		return
	}
	req.Symbol = normalizeRequestedSymbol(req.Symbol)

	auth := c.MustGet("auth").(forwardedAuth)
	response, err := d.runCopilotQuery(ctx, auth, c.ClientIP(), req, nil)
	if err != nil {
		status, payload := d.mapCopilotError(ctx, auth.UserID, err)
		c.JSON(status, payload)
		return
	}
	c.JSON(http.StatusOK, response)
}

func (d RouterDeps) streamCopilot(c *gin.Context) {
	ctx := c.Request.Context()
	var req domain.CopilotQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid copilot payload"})
		return
	}
	if strings.TrimSpace(req.Symbol) == "" || strings.TrimSpace(req.Question) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "symbol and question are required"})
		return
	}
	req.Symbol = normalizeRequestedSymbol(req.Symbol)

	auth := c.MustGet("auth").(forwardedAuth)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming unsupported"})
		return
	}
	emit := func(event string, payload any) error {
		if err := writeSSEEvent(c.Writer, event, payload); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	if err := emit("start", gin.H{}); err != nil {
		return
	}

	response, err := d.runCopilotStream(ctx, auth, c.ClientIP(), req, func(stage, message string) {
		_ = emit("stage", gin.H{
			"stage":   stage,
			"message": message,
		})
	}, func(content string) {
		_ = emit("delta", gin.H{"content": content})
	})
	if err != nil {
		_, payload := d.mapCopilotError(ctx, auth.UserID, err)
		_ = emit("error", payload)
		_ = emit("done", gin.H{"ok": false})
		return
	}

	_ = emit("result", response)
	_ = emit("done", gin.H{"ok": true})
}

func (d RouterDeps) runCopilotQuery(ctx context.Context, auth forwardedAuth, ip string, req domain.CopilotQueryRequest, onStage func(string, string)) (domain.CopilotQueryResponse, error) {
	if onStage != nil {
		onStage("loading_ohlc", "正在加载 K 线数据")
	}
	rows, err := d.repo.ListOHLC(ctx, req.Symbol, req.StartDate, req.EndDate)
	if err != nil {
		return domain.CopilotQueryResponse{}, fmt.Errorf("failed to load ohlc")
	}
	if len(rows) == 0 {
		if onStage != nil {
			onStage("syncing_symbol", "本地没有命中数据，正在同步最新行情")
		}
		if err := d.syncOneSymbol(ctx, req.Symbol); err != nil {
			return domain.CopilotQueryResponse{}, err
		}
		rows, err = d.repo.ListOHLC(ctx, req.Symbol, req.StartDate, req.EndDate)
		if err != nil {
			return domain.CopilotQueryResponse{}, fmt.Errorf("failed to load ohlc")
		}
	}

	if limited := d.enforceRateLimit(ctx, auth.UserID, ip); limited {
		return domain.CopilotQueryResponse{}, fmt.Errorf("ai rate limit exceeded")
	}

	providerID := strings.TrimSpace(strings.ToLower(req.Provider))
	if providerID == "" {
		for _, item := range d.copilot.Providers() {
			if item.IsDefault {
				providerID = item.ID
				break
			}
		}
	}
	if providerID == "" {
		providerID = "deepseek"
	}

	if !copilot.UsesUserAPIKey(req) {
		if onStage != nil {
			onStage("checking_allowance", "正在检查可用额度")
		}
		allowance, err := d.copilot.GetAIAllowanceStatus(ctx, auth.UserID)
		if err != nil {
			return domain.CopilotQueryResponse{}, fmt.Errorf("failed to load ai allowance")
		}
		if allowance.AvailableToConsume < copilot.CostForProvider(providerID) {
			return domain.CopilotQueryResponse{}, fmt.Errorf("insufficient ai allowance")
		}
	}

	response, _, err := d.copilot.QueryWithHooks(ctx, auth.UserID, req, rows, copilot.QueryHooks{
		OnFetchNews: func() {
			if onStage != nil {
				onStage("loading_news", "正在检索新闻证据")
			}
		},
		OnGenerateAI: func() {
			if onStage != nil {
				onStage("generating_answer", "正在生成分析结论")
			}
		},
		OnConsumeQuota: func() {
			if onStage != nil {
				onStage("consuming_allowance", "正在记录本次分析用量")
			}
		},
		OnSaveSession: func() {
			if onStage != nil {
				onStage("saving_session", "正在保存会话记录")
			}
		},
	})
	if err != nil {
		return domain.CopilotQueryResponse{}, err
	}
	_ = d.cache.Delete(
		ctx,
		sessionListCacheKey(auth.UserID, req.Symbol),
		sessionMessagesCacheKey(auth.UserID, response.SessionID),
		billingSummaryCacheKey(auth.UserID),
	)
	return response, nil
}

func (d RouterDeps) runCopilotStream(ctx context.Context, auth forwardedAuth, ip string, req domain.CopilotQueryRequest, onStage func(string, string), onDelta func(string)) (domain.CopilotQueryResponse, error) {
	if onStage != nil {
		onStage("loading_ohlc", "正在加载 K 线数据")
	}
	rows, err := d.repo.ListOHLC(ctx, req.Symbol, req.StartDate, req.EndDate)
	if err != nil {
		return domain.CopilotQueryResponse{}, fmt.Errorf("failed to load ohlc")
	}
	if len(rows) == 0 {
		if onStage != nil {
			onStage("syncing_symbol", "本地没有命中数据，正在同步最新行情")
		}
		if err := d.syncOneSymbol(ctx, req.Symbol); err != nil {
			return domain.CopilotQueryResponse{}, err
		}
		rows, err = d.repo.ListOHLC(ctx, req.Symbol, req.StartDate, req.EndDate)
		if err != nil {
			return domain.CopilotQueryResponse{}, fmt.Errorf("failed to load ohlc")
		}
	}

	if limited := d.enforceRateLimit(ctx, auth.UserID, ip); limited {
		return domain.CopilotQueryResponse{}, fmt.Errorf("ai rate limit exceeded")
	}

	providerID := strings.TrimSpace(strings.ToLower(req.Provider))
	if providerID == "" {
		for _, item := range d.copilot.Providers() {
			if item.IsDefault {
				providerID = item.ID
				break
			}
		}
	}
	if providerID == "" {
		providerID = "deepseek"
	}

	if !copilot.UsesUserAPIKey(req) {
		if onStage != nil {
			onStage("checking_allowance", "正在检查可用额度")
		}
		allowance, err := d.copilot.GetAIAllowanceStatus(ctx, auth.UserID)
		if err != nil {
			return domain.CopilotQueryResponse{}, fmt.Errorf("failed to load ai allowance")
		}
		if allowance.AvailableToConsume < copilot.CostForProvider(providerID) {
			return domain.CopilotQueryResponse{}, fmt.Errorf("insufficient ai allowance")
		}
	}

	response, _, err := d.copilot.QueryStreamWithHooks(ctx, auth.UserID, req, rows, copilot.QueryHooks{
		OnFetchNews: func() {
			if onStage != nil {
				onStage("loading_news", "正在检索新闻证据")
			}
		},
		OnGenerateAI: func() {
			if onStage != nil {
				onStage("generating_answer", "正在生成分析结论")
			}
		},
		OnConsumeQuota: func() {
			if onStage != nil {
				onStage("consuming_allowance", "正在记录本次分析用量")
			}
		},
		OnSaveSession: func() {
			if onStage != nil {
				onStage("saving_session", "正在保存会话记录")
			}
		},
		OnDelta: onDelta,
	})
	if err != nil {
		return domain.CopilotQueryResponse{}, err
	}

	_ = d.cache.Delete(
		ctx,
		sessionListCacheKey(auth.UserID, req.Symbol),
		sessionMessagesCacheKey(auth.UserID, response.SessionID),
		billingSummaryCacheKey(auth.UserID),
	)
	return response, nil
}

func (d RouterDeps) mapCopilotError(ctx context.Context, userID string, err error) (int, gin.H) {
	switch err.Error() {
	case "ai rate limit exceeded":
		return http.StatusTooManyRequests, gin.H{"error": "ai rate limit exceeded"}
	case "insufficient ai allowance":
		allowance, allowanceErr := d.copilot.GetAIAllowanceStatus(ctx, userID)
		if allowanceErr == nil {
			return http.StatusPaymentRequired, gin.H{
				"error":              "insufficient ai allowance",
				"today_quota":        allowance.TodayQuota,
				"credit_balance":     allowance.CreditBalance,
				"current_membership": allowance.CurrentMembership,
			}
		}
		return http.StatusPaymentRequired, gin.H{"error": "insufficient ai allowance"}
	case "failed to load ai allowance", "failed to load ohlc":
		return http.StatusInternalServerError, gin.H{"error": err.Error()}
	default:
		return http.StatusBadGateway, gin.H{"error": err.Error()}
	}
}

func writeSSEEvent(writer io.Writer, event string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "event: %s\n", event); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "data: %s\n\n", body); err != nil {
		return err
	}
	return nil
}

func splitAnswerChunks(answer string, chunkSize int) []string {
	if strings.TrimSpace(answer) == "" {
		return []string{}
	}
	runes := []rune(answer)
	result := make([]string, 0, (len(runes)/chunkSize)+1)
	for start := 0; start < len(runes); start += chunkSize {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		result = append(result, string(runes[start:end]))
	}
	return result
}

func (d RouterDeps) listCopilotSessions(c *gin.Context) {
	symbol := normalizeRequestedSymbol(c.Query("symbol"))
	if symbol == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "symbol is required"})
		return
	}

	auth := c.MustGet("auth").(forwardedAuth)
	cacheKey := sessionListCacheKey(auth.UserID, symbol)

	var cached []domain.AISessionSummary
	if hit, err := d.cache.GetJSON(c.Request.Context(), cacheKey, &cached); err == nil && hit {
		c.JSON(http.StatusOK, cached)
		return
	}

	sessions, err := d.copilot.ListSessions(c.Request.Context(), auth.UserID, symbol)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list copilot sessions"})
		return
	}

	_ = d.cache.SetJSON(c.Request.Context(), cacheKey, sessions, d.cfg.SessionCacheTTL)
	c.JSON(http.StatusOK, sessions)
}

func (d RouterDeps) getCopilotSessionMessages(c *gin.Context) {
	auth := c.MustGet("auth").(forwardedAuth)
	cacheKey := sessionMessagesCacheKey(auth.UserID, c.Param("id"))

	var cached domain.AISessionMessagesResponse
	if hit, err := d.cache.GetJSON(c.Request.Context(), cacheKey, &cached); err == nil && hit {
		c.JSON(http.StatusOK, cached)
		return
	}

	response, err := d.copilot.GetSessionMessages(c.Request.Context(), auth.UserID, c.Param("id"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load session messages"})
		return
	}

	_ = d.cache.SetJSON(c.Request.Context(), cacheKey, response, d.cfg.SessionCacheTTL)
	c.JSON(http.StatusOK, response)
}

func (d RouterDeps) toggleCopilotSessionFavorite(c *gin.Context) {
	auth := c.MustGet("auth").(forwardedAuth)
	var req domain.ToggleFavoriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid favorite payload"})
		return
	}

	sessionSymbol := ""
	if response, err := d.copilot.GetSessionMessages(c.Request.Context(), auth.UserID, c.Param("id")); err == nil {
		sessionSymbol = response.Session.Symbol
	}

	if err := d.copilot.ToggleSessionFavorite(c.Request.Context(), auth.UserID, c.Param("id"), req.IsFavorite); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update favorite"})
		return
	}

	_ = d.cache.Delete(
		c.Request.Context(),
		sessionListCacheKey(auth.UserID, sessionSymbol),
		sessionMessagesCacheKey(auth.UserID, c.Param("id")),
	)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (d RouterDeps) requireForwardedAuth(c *gin.Context) {
	userID := strings.TrimSpace(c.GetHeader("X-User-ID"))
	email := strings.TrimSpace(c.GetHeader("X-User-Email"))
	if userID == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing forwarded user"})
		return
	}

	c.Set("auth", forwardedAuth{
		UserID: userID,
		Email:  email,
	})
	c.Next()
}

func (d RouterDeps) syncOneSymbol(ctx context.Context, symbol string) error {
	stock, rows, err := d.eastmoney.FetchDailyOHLC(ctx, symbol)
	if err != nil {
		return err
	}
	return d.repo.UpsertSymbolWithRows(ctx, stock, rows)
}

func (d RouterDeps) enforceRateLimit(ctx context.Context, userID, ip string) bool {
	userKey := fmt.Sprintf("rate:ai:user:%s", userID)
	if count, err := d.cache.IncrementWindow(ctx, userKey, d.cfg.AIRateWindow); err == nil && count > int64(d.cfg.AIUserRateLimit) {
		return true
	}

	if ip == "" {
		return false
	}

	ipKey := fmt.Sprintf("rate:ai:ip:%s", ip)
	if count, err := d.cache.IncrementWindow(ctx, ipKey, d.cfg.AIRateWindow); err == nil && count > int64(d.cfg.AIIPRateLimit) {
		return true
	}

	return false
}

func normalizeRequestedSymbol(symbol string) string {
	symbol = strings.TrimSpace(strings.ToLower(symbol))
	symbol = strings.TrimPrefix(symbol, "sh")
	symbol = strings.TrimPrefix(symbol, "sz")
	return symbol
}
