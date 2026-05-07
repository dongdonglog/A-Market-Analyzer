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

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type CopilotQueryRequest struct {
	Symbol         string        `json:"symbol"`
	StartDate      string        `json:"start_date"`
	EndDate        string        `json:"end_date"`
	Provider       string        `json:"provider"`
	ProviderAPIKey string        `json:"provider_api_key,omitempty"`
	Question       string        `json:"question"`
	History        []ChatMessage `json:"history"`
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
	IsCompressed bool   `json:"is_compressed"`
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
