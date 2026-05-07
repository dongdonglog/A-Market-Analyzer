package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"market-project/backend/internal/cache"
	"market-project/backend/internal/config"
	"market-project/backend/internal/database"
	"market-project/backend/internal/domain"
	"market-project/backend/internal/services"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

type RouterDeps struct {
	cfg       config.Config
	cache     *cache.Client
	repo      *database.Repository
	eastmoney *services.EastmoneyClient
	aiURL     *url.URL
}

const symbolsCacheKey = "cache:symbols:list"

func tokenBlacklistKey(tokenID string) string {
	return fmt.Sprintf("auth:token:blacklist:%s", tokenID)
}

type authClaims struct {
	UserID  string `json:"user_id"`
	Email   string `json:"email"`
	IsAdmin bool   `json:"is_admin"`
	jwt.RegisteredClaims
}

func NewRouter(cfg config.Config, repo *database.Repository, eastmoney *services.EastmoneyClient, redisClient *cache.Client) *gin.Engine {
	aiURL, _ := url.Parse(cfg.AIServiceURL)
	deps := RouterDeps{
		cfg:       cfg,
		cache:     redisClient,
		repo:      repo,
		eastmoney: eastmoney,
		aiURL:     aiURL,
	}

	router := gin.Default()
	router.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CORSOrigins,
		AllowOriginFunc:  allowOrigin(cfg.CORSOrigins),
		AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowHeaders:     []string{"Authorization", "Content-Type"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	router.GET("/health", deps.health)
	router.POST("/auth/login", deps.login)
	router.POST("/auth/register", deps.register)

	secured := router.Group("/")
	secured.Use(deps.requireAuth)
	secured.POST("/auth/logout", deps.logout)
	secured.GET("/ai/providers", deps.proxyAIService)
	secured.GET("/symbols", deps.listSymbols)
	secured.POST("/symbols", deps.addSymbol)
	secured.GET("/symbols/search", deps.searchSymbols)
	secured.DELETE("/symbols/:symbol", deps.deleteSymbol)
	secured.GET("/symbols/:symbol/ohlc", deps.listOHLC)
	secured.GET("/copilot/sessions", deps.proxyAIService)
	secured.GET("/copilot/sessions/:id/messages", deps.proxyAIService)
	secured.POST("/copilot/sessions/:id/favorite", deps.proxyAIService)
	secured.POST("/copilot/query", deps.proxyAIService)
	secured.POST("/copilot/stream", deps.proxyAIService)

	return router
}

func (d RouterDeps) isAdminEmail(email string) bool {
	email = strings.TrimSpace(strings.ToLower(email))
	for _, item := range d.cfg.AdminEmails {
		if strings.TrimSpace(strings.ToLower(item)) == email {
			return true
		}
	}
	return false
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

		parsed, err := url.Parse(origin)
		if err != nil {
			return false
		}

		host := parsed.Hostname()
		return host == "localhost" || host == "127.0.0.1"
	}
}

func (d RouterDeps) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":          "ok",
		"tracked_symbols": d.cfg.TrackedSymbols,
		"timestamp":       time.Now().UTC().Format(time.RFC3339),
	})
}

func (d RouterDeps) login(c *gin.Context) {
	var req domain.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid login payload"})
		return
	}

	user, err := d.repo.FindUserByEmail(c.Request.Context(), req.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load user"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
		return
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	isAdmin := d.isAdminEmail(user.Email)
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, authClaims{
		UserID:  user.ID,
		Email:   user.Email,
		IsAdmin: isAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}).SignedString([]byte(d.cfg.JWTSecret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sign token"})
		return
	}

	c.JSON(http.StatusOK, domain.LoginResponse{
		Token: token,
		User: domain.User{
			ID:      user.ID,
			Email:   user.Email,
			IsAdmin: isAdmin,
		},
	})
}

func (d RouterDeps) listSymbols(c *gin.Context) {
	ctx := c.Request.Context()
	refresh := c.Query("refresh") == "1"
	if !refresh {
		var cached []domain.Symbol
		if hit, err := d.cache.GetJSON(ctx, symbolsCacheKey, &cached); err == nil && hit {
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	if refresh {
		if err := d.syncTrackedSymbols(ctx); err != nil {
			symbols, listErr := d.repo.ListSymbols(ctx)
			if listErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list symbols"})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"symbols": symbols,
				"warning": "market data refresh failed; returned cached symbols",
			})
			return
		}
	}

	symbols, err := d.repo.ListSymbols(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list symbols"})
		return
	}

	if len(symbols) == 0 {
		if err := d.syncTrackedSymbols(ctx); err != nil {
			c.JSON(http.StatusOK, []domain.Symbol{})
			return
		}
		symbols, err = d.repo.ListSymbols(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list symbols"})
			return
		}
	}

	_ = d.cache.SetJSON(ctx, symbolsCacheKey, symbols, d.cfg.SymbolsCacheTTL)
	c.JSON(http.StatusOK, symbols)
}

func (d RouterDeps) searchSymbols(c *gin.Context) {
	limit := 20
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		fmt.Sscanf(rawLimit, "%d", &limit)
	}

	symbols, err := d.repo.SearchSymbolCatalog(c.Request.Context(), c.Query("q"), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to search symbol catalog"})
		return
	}
	if symbols == nil {
		symbols = []domain.Symbol{}
	}

	c.JSON(http.StatusOK, symbols)
}

func (d RouterDeps) addSymbol(c *gin.Context) {
	var payload struct {
		Symbol string `json:"symbol"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid symbol payload"})
		return
	}

	symbol := normalizeRequestedSymbol(payload.Symbol)
	if len(symbol) != 6 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "symbol must be 6 digits"})
		return
	}

	record, err := d.repo.SearchSymbolCatalog(c.Request.Context(), symbol, 1)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to search symbol catalog"})
		return
	}

	toSave := inferSymbolRecord(symbol)
	if len(record) > 0 && record[0].Symbol == symbol {
		toSave = record[0]
	}

	if err := d.repo.UpsertSymbol(c.Request.Context(), toSave); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save symbol"})
		return
	}

	_ = d.cache.Delete(c.Request.Context(), symbolsCacheKey)
	c.JSON(http.StatusOK, toSave)
}

func (d RouterDeps) listOHLC(c *gin.Context) {
	ctx := c.Request.Context()
	symbol := normalizeRequestedSymbol(c.Param("symbol"))
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	cacheKey := fmt.Sprintf("cache:ohlc:%s:%s:%s", symbol, startDate, endDate)

	var cached []domain.OHLCRow
	if hit, err := d.cache.GetJSON(ctx, cacheKey, &cached); err == nil && hit {
		c.JSON(http.StatusOK, cached)
		return
	}

	rows, err := d.repo.ListOHLC(ctx, symbol, startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load ohlc"})
		return
	}

	alwaysSync := startDate == "" && endDate == ""
	if alwaysSync {
		syncCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		syncErr := d.syncOneSymbol(syncCtx, symbol)
		cancel()
		if syncErr == nil {
			_ = d.cache.Delete(ctx, cacheKey)
			rows, err = d.repo.ListOHLC(ctx, symbol, startDate, endDate)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load ohlc"})
				return
			}
		} else if len(rows) == 0 {
			c.JSON(http.StatusBadGateway, gin.H{"error": "market data refresh failed"})
			return
		}
	}

	_ = d.cache.SetJSON(ctx, cacheKey, rows, d.cfg.OHLCCacheTTL)
	c.JSON(http.StatusOK, rows)
}

func (d RouterDeps) deleteSymbol(c *gin.Context) {
	symbol := normalizeRequestedSymbol(c.Param("symbol"))
	if symbol == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "symbol is required"})
		return
	}
	if err := d.repo.DeleteSymbol(c.Request.Context(), symbol); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete symbol"})
		return
	}
	_ = d.cache.Delete(c.Request.Context(), symbolsCacheKey)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (d RouterDeps) proxyAIService(c *gin.Context) {
	if d.aiURL == nil || d.aiURL.Host == "" {
		c.JSON(http.StatusBadGateway, gin.H{"error": "ai service url is not configured"})
		return
	}

	claims := c.MustGet("auth").(authClaims)
	proxy := httputil.NewSingleHostReverseProxy(d.aiURL)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Header.Set("X-User-ID", claims.UserID)
		req.Header.Set("X-User-Email", claims.Email)
	}
	proxy.ErrorHandler = func(writer http.ResponseWriter, request *http.Request, err error) {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to reach ai service"})
	}

	proxy.ServeHTTP(c.Writer, c.Request)
}

func (d RouterDeps) logout(c *gin.Context) {
	claims := c.MustGet("auth").(authClaims)
	tokenString := c.MustGet("token").(string)
	if claims.ID != "" && claims.ExpiresAt != nil {
		ttl := time.Until(claims.ExpiresAt.Time)
		if ttl > 0 {
			_ = d.cache.SetString(c.Request.Context(), tokenBlacklistKey(claims.ID), tokenString, ttl)
		}
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (d RouterDeps) register(c *gin.Context) {
	var req domain.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid register payload"})
		return
	}
	if strings.TrimSpace(req.Email) == "" || len(strings.TrimSpace(req.Password)) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email and password(>=6) are required"})
		return
	}
	user, err := d.repo.CreateUser(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "email already exists"})
		return
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	isAdmin := d.isAdminEmail(user.Email)
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, authClaims{
		UserID:  user.ID,
		Email:   user.Email,
		IsAdmin: isAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}).SignedString([]byte(d.cfg.JWTSecret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sign token"})
		return
	}

	c.JSON(http.StatusOK, domain.LoginResponse{
		Token: token,
		User:  domain.User{ID: user.ID, Email: user.Email, IsAdmin: isAdmin},
	})
}

func (d RouterDeps) requireAuth(c *gin.Context) {
	header := c.GetHeader("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
		return
	}

	tokenString := strings.TrimPrefix(header, "Bearer ")
	token, err := jwt.ParseWithClaims(tokenString, &authClaims{}, func(token *jwt.Token) (any, error) {
		return []byte(d.cfg.JWTSecret), nil
	})
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	claims, ok := token.Claims.(*authClaims)
	if !ok || !token.Valid {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
		return
	}

	if claims.ID != "" {
		blacklisted, err := d.cache.Exists(c.Request.Context(), tokenBlacklistKey(claims.ID))
		if err == nil && blacklisted {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token revoked"})
			return
		}
	}

	c.Set("auth", *claims)
	c.Set("token", tokenString)
	c.Next()
}

func (d RouterDeps) syncTrackedSymbols(ctx context.Context) error {
	for _, symbol := range d.cfg.TrackedSymbols {
		if err := d.syncOneSymbol(ctx, symbol); err != nil {
			return err
		}
	}
	_ = d.cache.Delete(ctx, symbolsCacheKey)
	return nil
}

func (d RouterDeps) syncOneSymbol(ctx context.Context, symbol string) error {
	stock, rows, err := d.eastmoney.FetchDailyOHLC(ctx, symbol)
	if err != nil {
		return err
	}
	if stock.Name == "" || stock.Name == stock.Symbol {
		if catalogRecord, catalogErr := d.repo.FindSymbolCatalogByCode(ctx, stock.Symbol); catalogErr == nil {
			stock.Name = catalogRecord.Name
			if catalogRecord.Market != "" {
				stock.Market = catalogRecord.Market
			}
		}
	}
	if err := d.repo.UpsertSymbolWithRows(ctx, stock, rows); err != nil {
		return err
	}
	_ = d.cache.Delete(ctx, symbolsCacheKey)
	return nil
}

func normalizeRequestedSymbol(symbol string) string {
	symbol = strings.TrimSpace(strings.ToLower(symbol))
	symbol = strings.TrimPrefix(symbol, "sh")
	symbol = strings.TrimPrefix(symbol, "sz")
	return symbol
}

func inferSymbolRecord(symbol string) domain.Symbol {
	market := "SZ"
	if strings.HasPrefix(symbol, "6") || strings.HasPrefix(symbol, "5") || strings.HasPrefix(symbol, "9") {
		market = "SH"
	}
	return domain.Symbol{
		Symbol: symbol,
		Name:   symbol,
		Market: market,
		Source: "manual",
	}
}
