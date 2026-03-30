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

func billingSummaryCacheKey(userID string) string {
	return fmt.Sprintf("cache:billing:user:%s", userID)
}

func mockPaymentURL(orderID string) string {
	return fmt.Sprintf("mock-alipay://scan/%s", orderID)
}

func mockPaymentQRCode(orderID string) string {
	return fmt.Sprintf("MOCK-ALIPAY-QR:%s", orderID)
}

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
	router.POST("/payments/alipay/mock/notify", deps.mockAlipayNotify)

	secured := router.Group("/")
	secured.Use(deps.requireAuth)
	secured.POST("/auth/logout", deps.logout)
	secured.GET("/ai/providers", deps.proxyAIService)
	secured.GET("/symbols", deps.listSymbols)
	secured.GET("/symbols/search", deps.searchSymbols)
	secured.DELETE("/symbols/:symbol", deps.deleteSymbol)
	secured.GET("/symbols/:symbol/ohlc", deps.listOHLC)
	secured.GET("/copilot/sessions", deps.proxyAIService)
	secured.GET("/copilot/sessions/:id/messages", deps.proxyAIService)
	secured.POST("/copilot/sessions/:id/favorite", deps.proxyAIService)
	secured.POST("/copilot/query", deps.proxyAIService)
	secured.POST("/copilot/stream", deps.proxyAIService)
	secured.GET("/billing/summary", deps.billingSummary)
	secured.POST("/billing/recharge-orders", deps.createRechargeOrder)
	secured.POST("/billing/recharge-orders/:id/mock-pay", deps.mockPayRechargeOrder)
	secured.POST("/billing/redeem-codes/redeem", deps.redeemCode)

	admin := secured.Group("/admin")
	admin.Use(deps.requireAdmin)
	admin.GET("/redeem-codes", deps.listAdminRedeemCodes)
	admin.GET("/redeem-code-claims", deps.listAdminRedeemCodeClaims)
	admin.GET("/action-logs", deps.listAdminActionLogs)
	admin.GET("/users", deps.listAdminUsers)
	admin.GET("/users/:id", deps.getAdminUserDetail)
	admin.POST("/users/:id/bonus-credits", deps.adminGrantBonusCredits)
	admin.POST("/users/:id/memberships", deps.adminGrantMembership)
	admin.POST("/redeem-codes", deps.createAdminRedeemCode)
	admin.POST("/redeem-codes/batch", deps.batchCreateAdminRedeemCodes)
	admin.POST("/redeem-codes/:code/disable", deps.disableAdminRedeemCode)

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
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
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
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
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

	if len(rows) == 0 {
		if err := d.syncOneSymbol(ctx, symbol); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		rows, err = d.repo.ListOHLC(ctx, symbol, startDate, endDate)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load ohlc"})
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

func billingPackages() []domain.BillingPackage {
	return []domain.BillingPackage{
		{ID: "starter", Name: "Starter", AmountCNY: 29, DailyQuota: 10000, DurationDays: 30, Description: "适合试用与轻度分析，每日额度 10,000"},
		{ID: "active", Name: "Active", AmountCNY: 99, DailyQuota: 40000, DurationDays: 30, Description: "适合日常看盘与多轮追问，每日额度 40,000"},
		{ID: "pro", Name: "Pro", AmountCNY: 299, DailyQuota: 140000, DurationDays: 30, Description: "适合高频分析与重度使用，每日额度 140,000"},
	}
}

func findBillingPackage(packageID string) (domain.BillingPackage, bool) {
	for _, pkg := range billingPackages() {
		if pkg.ID == packageID {
			return pkg, true
		}
	}
	return domain.BillingPackage{}, false
}

func (d RouterDeps) billingSummary(c *gin.Context) {
	claims := c.MustGet("auth").(authClaims)
	cacheKey := billingSummaryCacheKey(claims.UserID)

	var cached domain.BillingSummary
	if hit, err := d.cache.GetJSON(c.Request.Context(), cacheKey, &cached); err == nil && hit {
		c.JSON(http.StatusOK, cached)
		return
	}

	allowance, err := d.repo.GetAIAllowanceStatus(c.Request.Context(), claims.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load balance"})
		return
	}
	orders, err := d.repo.ListRechargeOrders(c.Request.Context(), claims.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load orders"})
		return
	}
	usage, err := d.repo.ListUsageRecords(c.Request.Context(), claims.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load usage"})
		return
	}

	response := domain.BillingSummary{
		CreditBalance:     allowance.CreditBalance,
		CurrentMembership: allowance.CurrentMembership,
		TodayQuota:        allowance.TodayQuota,
		Packages:          billingPackages(),
		Orders:            coalesceOrders(orders),
		Usage:             coalesceUsage(usage),
	}
	_ = d.cache.SetJSON(c.Request.Context(), cacheKey, response, d.cfg.BillingCacheTTL)
	c.JSON(http.StatusOK, response)
}

func coalesceOrders(input []domain.RechargeOrderResponse) []domain.RechargeOrderResponse {
	if input == nil {
		return []domain.RechargeOrderResponse{}
	}
	return input
}

func coalesceUsage(input []domain.UsageRecord) []domain.UsageRecord {
	if input == nil {
		return []domain.UsageRecord{}
	}
	return input
}

func (d RouterDeps) createRechargeOrder(c *gin.Context) {
	claims := c.MustGet("auth").(authClaims)
	var req domain.RechargeOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid recharge payload"})
		return
	}

	pkg, ok := findBillingPackage(strings.TrimSpace(req.PackageID))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown package"})
		return
	}

	method := strings.TrimSpace(strings.ToLower(req.PaymentMethod))
	if method != "alipay" && method != "wechat" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "payment_method must be alipay or wechat"})
		return
	}

	order, err := d.repo.CreateRechargeOrder(c.Request.Context(), claims.UserID, pkg, method, "", "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create recharge order"})
		return
	}
	_ = d.cache.Delete(c.Request.Context(), billingSummaryCacheKey(claims.UserID))

	if method == "alipay" && d.cfg.MockPayments {
		order.PaymentURL = mockPaymentURL(order.OrderID)
		order.QRCode = mockPaymentQRCode(order.OrderID)
		order.MockPayReady = true
		order.PayHint = "当前为支付宝扫码 mock 流程。点击模拟支付后会触发 mock 回调并激活会员。"
	} else {
		order.PayHint = "订单已创建，等待真实支付链路接入。"
	}
	c.JSON(http.StatusOK, order)
}

func (d RouterDeps) mockPayRechargeOrder(c *gin.Context) {
	if !d.cfg.MockPayments {
		c.JSON(http.StatusForbidden, gin.H{"error": "mock payments disabled"})
		return
	}

	claims := c.MustGet("auth").(authClaims)
	order, err := d.repo.GetRechargeOrder(c.Request.Context(), claims.UserID, c.Param("id"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "order not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load order"})
		return
	}
	if order.PaymentMethod != "alipay" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mock pay only supports alipay orders"})
		return
	}

	userID, err := d.repo.MarkRechargeOrderPaid(c.Request.Context(), order.OrderID, "mock_alipay_"+uuid.NewString())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mark order paid"})
		return
	}
	_ = d.cache.Delete(c.Request.Context(), billingSummaryCacheKey(userID))

	updated, err := d.repo.GetRechargeOrder(c.Request.Context(), claims.UserID, order.OrderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reload order"})
		return
	}
	updated.PayHint = "mock 支付已完成，会员已激活。"
	c.JSON(http.StatusOK, updated)
}

func (d RouterDeps) mockAlipayNotify(c *gin.Context) {
	if !d.cfg.MockPayments {
		c.JSON(http.StatusForbidden, gin.H{"error": "mock payments disabled"})
		return
	}

	orderID := strings.TrimSpace(c.Query("order_id"))
	if orderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "order_id is required"})
		return
	}

	userID, err := d.repo.MarkRechargeOrderPaid(c.Request.Context(), orderID, "mock_notify_"+uuid.NewString())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "order not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mark order paid"})
		return
	}
	_ = d.cache.Delete(c.Request.Context(), billingSummaryCacheKey(userID))
	c.JSON(http.StatusOK, gin.H{"ok": true, "order_id": orderID})
}

func (d RouterDeps) redeemCode(c *gin.Context) {
	claims := c.MustGet("auth").(authClaims)
	var req domain.RedeemCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Code) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	response, err := d.repo.ClaimRedeemCode(c.Request.Context(), claims.UserID, req.Code)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_ = d.cache.Delete(c.Request.Context(), billingSummaryCacheKey(claims.UserID))
	c.JSON(http.StatusOK, response)
}

func (d RouterDeps) listAdminRedeemCodes(c *gin.Context) {
	items, err := d.repo.ListAdminRedeemCodes(
		c.Request.Context(),
		c.Query("search"),
		c.Query("reward_type"),
		c.Query("status"),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list redeem codes"})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (d RouterDeps) createAdminRedeemCode(c *gin.Context) {
	claims := c.MustGet("auth").(authClaims)
	var req domain.AdminCreateRedeemCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid redeem code payload"})
		return
	}

	rewardType := strings.TrimSpace(strings.ToLower(req.RewardType))
	if rewardType == "" {
		rewardType = "membership"
	}

	code := strings.TrimSpace(strings.ToUpper(req.Code))
	if code == "" {
		code = "CODE-" + strings.ToUpper(strings.ReplaceAll(uuid.NewString()[:8], "-", ""))
	}

	maxClaims := req.MaxClaims
	if maxClaims <= 0 {
		maxClaims = 1
	}

	item := domain.AdminRedeemCode{
		Code:       code,
		RewardType: rewardType,
		MaxClaims:  maxClaims,
		IsActive:   true,
	}

	if strings.TrimSpace(req.ExpiresAt) != "" {
		expiresAt, err := parseAdminExpiresAt(req.ExpiresAt)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid expires_at"})
			return
		}
		item.ExpiresAt = expiresAt.Format(time.RFC3339)
	}

	switch rewardType {
	case "membership":
		pkg, ok := findBillingPackage(strings.TrimSpace(req.PackageID))
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown package"})
			return
		}
		item.PackageID = pkg.ID
		item.PackageName = pkg.Name
		item.DailyQuota = pkg.DailyQuota
		item.DurationDays = pkg.DurationDays
	case "bonus_credits":
		if req.BonusCredits <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bonus_credits must be positive"})
			return
		}
		item.BonusCredits = req.BonusCredits
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "reward_type must be membership or bonus_credits"})
		return
	}

	created, err := d.repo.CreateAdminRedeemCode(c.Request.Context(), item)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "failed to create redeem code"})
		return
	}
	_ = d.repo.LogAdminAction(
		c.Request.Context(),
		claims.UserID,
		claims.Email,
		"create_redeem_code",
		"redeem_code",
		created.Code,
		fmt.Sprintf("created %s redeem code", created.RewardType),
	)
	c.JSON(http.StatusOK, created)
}

func (d RouterDeps) batchCreateAdminRedeemCodes(c *gin.Context) {
	claims := c.MustGet("auth").(authClaims)
	var req domain.AdminBatchCreateRedeemCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid batch redeem code payload"})
		return
	}

	count := req.Count
	if count <= 0 || count > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "count must be between 1 and 100"})
		return
	}

	maxClaims := req.MaxClaims
	if maxClaims <= 0 {
		maxClaims = 1
	}

	rewardType := strings.TrimSpace(strings.ToLower(req.RewardType))
	if rewardType == "" {
		rewardType = "membership"
	}

	prefix := strings.TrimSpace(strings.ToUpper(req.Prefix))
	if prefix == "" {
		prefix = "BATCH"
	}

	itemBase := domain.AdminRedeemCode{
		RewardType: rewardType,
		MaxClaims:  maxClaims,
		IsActive:   true,
	}

	if strings.TrimSpace(req.ExpiresAt) != "" {
		expiresAt, err := parseAdminExpiresAt(req.ExpiresAt)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid expires_at"})
			return
		}
		itemBase.ExpiresAt = expiresAt.Format(time.RFC3339)
	}

	switch rewardType {
	case "membership":
		pkg, ok := findBillingPackage(strings.TrimSpace(req.PackageID))
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown package"})
			return
		}
		itemBase.PackageID = pkg.ID
		itemBase.PackageName = pkg.Name
		itemBase.DailyQuota = pkg.DailyQuota
		itemBase.DurationDays = pkg.DurationDays
	case "bonus_credits":
		if req.BonusCredits <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bonus_credits must be positive"})
			return
		}
		itemBase.BonusCredits = req.BonusCredits
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "reward_type must be membership or bonus_credits"})
		return
	}

	result := make([]domain.AdminRedeemCode, 0, count)
	for i := 0; i < count; i++ {
		item := itemBase
		item.Code = fmt.Sprintf("%s-%s", prefix, strings.ToUpper(strings.ReplaceAll(uuid.NewString()[:8], "-", "")))
		created, err := d.repo.CreateAdminRedeemCode(c.Request.Context(), item)
		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "failed to create redeem code batch"})
			return
		}
		result = append(result, created)
	}

	_ = d.repo.LogAdminAction(
		c.Request.Context(),
		claims.UserID,
		claims.Email,
		"batch_create_redeem_codes",
		"redeem_code_batch",
		prefix,
		fmt.Sprintf("created %d %s codes", len(result), rewardType),
	)

	c.JSON(http.StatusOK, result)
}

func parseAdminExpiresAt(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}

	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed, nil
	}

	return time.ParseInLocation("2006-01-02T15:04", raw, time.Local)
}

func (d RouterDeps) disableAdminRedeemCode(c *gin.Context) {
	claims := c.MustGet("auth").(authClaims)
	code := strings.TrimSpace(c.Param("code"))
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	if err := d.repo.DisableAdminRedeemCode(c.Request.Context(), code); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "redeem code not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to disable redeem code"})
		return
	}
	_ = d.repo.LogAdminAction(
		c.Request.Context(),
		claims.UserID,
		claims.Email,
		"disable_redeem_code",
		"redeem_code",
		strings.ToUpper(code),
		"disabled redeem code",
	)
	c.JSON(http.StatusOK, gin.H{"ok": true, "code": strings.ToUpper(code)})
}

func (d RouterDeps) listAdminRedeemCodeClaims(c *gin.Context) {
	items, err := d.repo.ListAdminRedeemCodeClaims(c.Request.Context(), c.Query("search"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list redeem code claims"})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (d RouterDeps) listAdminActionLogs(c *gin.Context) {
	items, err := d.repo.ListAdminActionLogs(c.Request.Context(), c.Query("search"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list action logs"})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (d RouterDeps) listAdminUsers(c *gin.Context) {
	items, err := d.repo.ListAdminUsers(
		c.Request.Context(),
		d.cfg.AdminEmails,
		c.Query("search"),
		c.Query("membership_status"),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list users"})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (d RouterDeps) getAdminUserDetail(c *gin.Context) {
	userID := strings.TrimSpace(c.Param("id"))
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user id is required"})
		return
	}

	summary, err := d.repo.GetAdminUserSummary(c.Request.Context(), d.cfg.AdminEmails, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load user"})
		return
	}

	allowance, err := d.repo.GetAIAllowanceStatus(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load user allowance"})
		return
	}

	usage, err := d.repo.ListUsageRecords(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load user usage"})
		return
	}

	memberships, err := d.repo.ListAdminUserMemberships(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load membership history"})
		return
	}

	redeemClaims, err := d.repo.ListAdminUserRedeemClaims(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load redeem claims"})
		return
	}

	c.JSON(http.StatusOK, domain.AdminUserDetail{
		UserID:            summary.UserID,
		Email:             summary.Email,
		IsAdmin:           summary.IsAdmin,
		CreditBalance:     allowance.CreditBalance,
		CreatedAt:         summary.CreatedAt,
		CurrentMembership: allowance.CurrentMembership,
		TodayQuota:        allowance.TodayQuota,
		RecentUsage:       coalesceUsage(usage),
		Memberships:       coalesceAdminMemberships(memberships),
		RedeemClaims:      coalesceAdminRedeemClaims(redeemClaims),
	})
}

func (d RouterDeps) adminGrantBonusCredits(c *gin.Context) {
	claims := c.MustGet("auth").(authClaims)
	userID := strings.TrimSpace(c.Param("id"))
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user id is required"})
		return
	}

	var req domain.AdminGrantCreditsRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Amount <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "amount must be positive"})
		return
	}

	balance, err := d.repo.AddAdminBonusCredits(c.Request.Context(), userID, req.Amount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to grant bonus credits"})
		return
	}

	_ = d.cache.Delete(c.Request.Context(), billingSummaryCacheKey(userID))
	_ = d.repo.LogAdminAction(
		c.Request.Context(),
		claims.UserID,
		claims.Email,
		"grant_bonus_credits",
		"user",
		userID,
		fmt.Sprintf("granted %d bonus credits", req.Amount),
	)
	c.JSON(http.StatusOK, gin.H{"ok": true, "credit_balance": balance})
}

func (d RouterDeps) adminGrantMembership(c *gin.Context) {
	claims := c.MustGet("auth").(authClaims)
	userID := strings.TrimSpace(c.Param("id"))
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user id is required"})
		return
	}

	var req domain.AdminGrantMembershipRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid membership payload"})
		return
	}

	pkg, ok := findBillingPackage(strings.TrimSpace(req.PackageID))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown package"})
		return
	}

	membership, err := d.repo.GrantAdminMembership(c.Request.Context(), userID, pkg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to grant membership"})
		return
	}

	_ = d.cache.Delete(c.Request.Context(), billingSummaryCacheKey(userID))
	_ = d.repo.LogAdminAction(
		c.Request.Context(),
		claims.UserID,
		claims.Email,
		"grant_membership",
		"user",
		userID,
		fmt.Sprintf("granted %s membership", pkg.Name),
	)
	c.JSON(http.StatusOK, membership)
}

func coalesceAdminMemberships(input []domain.AdminUserMembershipRecord) []domain.AdminUserMembershipRecord {
	if input == nil {
		return []domain.AdminUserMembershipRecord{}
	}
	return input
}

func coalesceAdminRedeemClaims(input []domain.AdminUserRedeemClaim) []domain.AdminUserRedeemClaim {
	if input == nil {
		return []domain.AdminUserRedeemClaim{}
	}
	return input
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

func (d RouterDeps) requireAdmin(c *gin.Context) {
	claims := c.MustGet("auth").(authClaims)
	if !claims.IsAdmin {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin access required"})
		return
	}
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

func buildSessionSummary(question, answer string) (string, string) {
	question = strings.TrimSpace(question)
	answer = strings.TrimSpace(answer)
	title := question
	if title == "" {
		title = "Untitled Session"
	}

	summary := question
	if answer != "" {
		summary = answer
	}

	titleRunes := []rune(title)
	if len(titleRunes) > 28 {
		title = string(titleRunes[:28]) + "..."
	}

	runes := []rune(summary)
	if len(runes) > 96 {
		summary = string(runes[:96]) + "..."
	}
	return title, summary
}
