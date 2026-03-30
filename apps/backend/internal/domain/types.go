package domain

type User struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	IsAdmin      bool   `json:"is_admin"`
	PasswordHash string `json:"-"`
}

type Symbol struct {
	Symbol string `json:"symbol"`
	Name   string `json:"name"`
	Market string `json:"market"`
	Source string `json:"source"`
}

type OHLCRow struct {
	Symbol     string   `json:"symbol"`
	Market     string   `json:"market"`
	Date       string   `json:"date"`
	Open       float64  `json:"open"`
	High       float64  `json:"high"`
	Low        float64  `json:"low"`
	Close      float64  `json:"close"`
	Volume     float64  `json:"volume"`
	Amount     *float64 `json:"amount,omitempty"`
	ChangeRate *float64 `json:"change_rate,omitempty"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

type CopilotQueryRequest struct {
	Symbol    string        `json:"symbol"`
	StartDate string        `json:"start_date"`
	EndDate   string        `json:"end_date"`
	Provider  string        `json:"provider"`
	Question  string        `json:"question"`
	History   []ChatMessage `json:"history"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type CopilotQueryResponse struct {
	SessionID   string             `json:"session_id,omitempty"`
	SessionDate string             `json:"session_date,omitempty"`
	Answer      string             `json:"answer"`
	Bias        string             `json:"bias"`
	KeyPoints   []string           `json:"key_points"`
	RiskPoints  []string           `json:"risk_points"`
	WatchItems  []string           `json:"watch_items"`
	Levels      CopilotLevels      `json:"levels"`
	NewsContext CopilotNewsContext `json:"news_context"`
}

type CopilotLevel struct {
	Value  float64 `json:"value"`
	Reason string  `json:"reason"`
}

type CopilotLevels struct {
	Support  CopilotLevel `json:"support"`
	Pressure CopilotLevel `json:"pressure"`
	Risk     CopilotLevel `json:"risk"`
}

type CopilotNewsItem struct {
	Title           string `json:"title"`
	Source          string `json:"source"`
	PublishedAt     string `json:"published_at"`
	URL             string `json:"url"`
	Summary         string `json:"summary"`
	RelevanceReason string `json:"relevance_reason"`
}

type CopilotNewsContext struct {
	Used  bool              `json:"used"`
	Count int               `json:"count"`
	Items []CopilotNewsItem `json:"items"`
	Note  string            `json:"note,omitempty"`
}

type AISessionSummary struct {
	ID           string `json:"id"`
	SessionDate  string `json:"session_date"`
	Symbol       string `json:"symbol"`
	MessageCount int    `json:"message_count"`
	UpdatedAt    string `json:"updated_at"`
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	IsFavorite   bool   `json:"is_favorite"`
}

type AISessionMessagesResponse struct {
	Session  AISessionSummary `json:"session"`
	Messages []ChatMessage    `json:"messages"`
}

type ToggleFavoriteRequest struct {
	IsFavorite bool `json:"is_favorite"`
}

type AIProviderInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Model     string `json:"model"`
	Enabled   bool   `json:"enabled"`
	IsDefault bool   `json:"is_default"`
}

type BillingPackage struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	AmountCNY    int    `json:"amount_cny"`
	DailyQuota   int    `json:"daily_quota"`
	DurationDays int    `json:"duration_days"`
	Description  string `json:"description"`
}

type MembershipStatus struct {
	PackageID    string `json:"package_id"`
	PackageName  string `json:"package_name"`
	Status       string `json:"status"`
	DailyQuota   int    `json:"daily_quota"`
	DurationDays int    `json:"duration_days"`
	StartsAt     string `json:"starts_at"`
	EndsAt       string `json:"ends_at"`
}

type DailyQuotaStatus struct {
	Date      string `json:"date"`
	Total     int    `json:"total"`
	Used      int    `json:"used"`
	Remaining int    `json:"remaining"`
}

type BillingSummary struct {
	CreditBalance     int                     `json:"credit_balance"`
	CurrentMembership *MembershipStatus       `json:"current_membership,omitempty"`
	TodayQuota        DailyQuotaStatus        `json:"today_quota"`
	Packages          []BillingPackage        `json:"packages"`
	Orders            []RechargeOrderResponse `json:"orders"`
	Usage             []UsageRecord           `json:"usage"`
}

type RechargeOrderRequest struct {
	PackageID     string `json:"package_id"`
	PaymentMethod string `json:"payment_method"`
}

type RechargeOrderResponse struct {
	OrderID       string `json:"order_id"`
	Status        string `json:"status"`
	PackageID     string `json:"package_id"`
	PaymentMethod string `json:"payment_method"`
	AmountCNY     int    `json:"amount_cny"`
	DailyQuota    int    `json:"daily_quota"`
	DurationDays  int    `json:"duration_days"`
	PaymentURL    string `json:"payment_url,omitempty"`
	QRCode        string `json:"qr_code,omitempty"`
	MockPayReady  bool   `json:"mock_pay_ready"`
	PayHint       string `json:"pay_hint"`
}

type UsageRecord struct {
	ID          string `json:"id"`
	Provider    string `json:"provider"`
	Symbol      string `json:"symbol"`
	CostCredits int    `json:"cost_credits"`
	QuotaUsed   int    `json:"quota_used"`
	BonusUsed   int    `json:"bonus_used"`
	CreatedAt   string `json:"created_at"`
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RedeemCodeRequest struct {
	Code string `json:"code"`
}

type RedeemCodeResponse struct {
	Code                string            `json:"code"`
	RewardType          string            `json:"reward_type"`
	BonusCredits        int               `json:"bonus_credits"`
	CreditBalance       int               `json:"credit_balance"`
	ActivatedMembership *MembershipStatus `json:"activated_membership,omitempty"`
	Message             string            `json:"message"`
}

type AIAllowanceStatus struct {
	CreditBalance      int               `json:"credit_balance"`
	CurrentMembership  *MembershipStatus `json:"current_membership,omitempty"`
	TodayQuota         DailyQuotaStatus  `json:"today_quota"`
	AvailableToConsume int               `json:"available_to_consume"`
}

type AdminCreateRedeemCodeRequest struct {
	Code         string `json:"code"`
	RewardType   string `json:"reward_type"`
	BonusCredits int    `json:"bonus_credits"`
	PackageID    string `json:"package_id"`
	MaxClaims    int    `json:"max_claims"`
	ExpiresAt    string `json:"expires_at"`
}

type AdminBatchCreateRedeemCodeRequest struct {
	Prefix       string `json:"prefix"`
	Count        int    `json:"count"`
	RewardType   string `json:"reward_type"`
	BonusCredits int    `json:"bonus_credits"`
	PackageID    string `json:"package_id"`
	MaxClaims    int    `json:"max_claims"`
	ExpiresAt    string `json:"expires_at"`
}

type AdminRedeemCode struct {
	Code         string `json:"code"`
	RewardType   string `json:"reward_type"`
	BonusCredits int    `json:"bonus_credits"`
	PackageID    string `json:"package_id"`
	PackageName  string `json:"package_name"`
	DailyQuota   int    `json:"daily_quota"`
	DurationDays int    `json:"duration_days"`
	MaxClaims    int    `json:"max_claims"`
	ClaimedCount int    `json:"claimed_count"`
	IsActive     bool   `json:"is_active"`
	ExpiresAt    string `json:"expires_at"`
	CreatedAt    string `json:"created_at"`
}

type AdminRedeemCodeClaim struct {
	Code         string `json:"code"`
	RewardType   string `json:"reward_type"`
	UserEmail    string `json:"user_email"`
	BonusCredits int    `json:"bonus_credits"`
	PackageName  string `json:"package_name"`
	CreatedAt    string `json:"created_at"`
}

type AdminUserSummary struct {
	UserID           string `json:"user_id"`
	Email            string `json:"email"`
	IsAdmin          bool   `json:"is_admin"`
	CreditBalance    int    `json:"credit_balance"`
	CurrentPackage   string `json:"current_package"`
	DailyQuota       int    `json:"daily_quota"`
	MembershipEndsAt string `json:"membership_ends_at"`
	CreatedAt        string `json:"created_at"`
}

type AdminUserMembershipRecord struct {
	ID           string `json:"id"`
	PackageID    string `json:"package_id"`
	PackageName  string `json:"package_name"`
	Status       string `json:"status"`
	DailyQuota   int    `json:"daily_quota"`
	DurationDays int    `json:"duration_days"`
	StartsAt     string `json:"starts_at"`
	EndsAt       string `json:"ends_at"`
	CreatedAt    string `json:"created_at"`
}

type AdminUserRedeemClaim struct {
	Code         string `json:"code"`
	RewardType   string `json:"reward_type"`
	BonusCredits int    `json:"bonus_credits"`
	PackageName  string `json:"package_name"`
	CreatedAt    string `json:"created_at"`
}

type AdminUserDetail struct {
	UserID            string                      `json:"user_id"`
	Email             string                      `json:"email"`
	IsAdmin           bool                        `json:"is_admin"`
	CreditBalance     int                         `json:"credit_balance"`
	CreatedAt         string                      `json:"created_at"`
	CurrentMembership *MembershipStatus           `json:"current_membership,omitempty"`
	TodayQuota        DailyQuotaStatus            `json:"today_quota"`
	RecentUsage       []UsageRecord               `json:"recent_usage"`
	Memberships       []AdminUserMembershipRecord `json:"memberships"`
	RedeemClaims      []AdminUserRedeemClaim      `json:"redeem_claims"`
}

type AdminGrantCreditsRequest struct {
	Amount int `json:"amount"`
}

type AdminGrantMembershipRequest struct {
	PackageID string `json:"package_id"`
}

type AdminActionLog struct {
	ID          string `json:"id"`
	AdminEmail  string `json:"admin_email"`
	ActionType  string `json:"action_type"`
	TargetType  string `json:"target_type"`
	TargetID    string `json:"target_id"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
}
