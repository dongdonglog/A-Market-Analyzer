package copilot

import (
	"context"
	"strings"

	"market-project/backend/internal/database"
	"market-project/backend/internal/domain"
	"market-project/backend/internal/services"
)

type Service struct {
	repo *database.Repository
	news *services.NewsClient
	ai   *services.AIClient
}

type QueryHooks struct {
	OnFetchNews   func()
	OnGenerateAI  func()
	OnSaveSession func()
	OnDelta       func(string)
}

func NewService(repo *database.Repository, news *services.NewsClient, ai *services.AIClient) *Service {
	return &Service{
		repo: repo,
		news: news,
		ai:   ai,
	}
}

func (s *Service) Providers() []domain.AIProviderInfo {
	return s.ai.Providers()
}

func UsesUserAPIKey(req domain.CopilotQueryRequest) bool {
	return strings.TrimSpace(req.ProviderAPIKey) != ""
}

func (s *Service) Query(ctx context.Context, userID string, req domain.CopilotQueryRequest, rows []domain.OHLCRow) (domain.CopilotQueryResponse, string, error) {
	return s.QueryWithHooks(ctx, userID, req, rows, QueryHooks{})
}

func (s *Service) QueryWithHooks(ctx context.Context, userID string, req domain.CopilotQueryRequest, rows []domain.OHLCRow, hooks QueryHooks) (domain.CopilotQueryResponse, string, error) {
	symbolRecord, _ := s.repo.FindSymbol(ctx, req.Symbol)
	newsKeyword := symbolRecord.Name
	if newsKeyword == "" {
		newsKeyword = req.Symbol
	}
	if hooks.OnFetchNews != nil {
		hooks.OnFetchNews()
	}
	newsItems, _ := s.news.SearchSymbolNews(ctx, newsKeyword, req.StartDate, req.EndDate, 5)

	providerID := strings.TrimSpace(strings.ToLower(req.Provider))
	if providerID == "" {
		for _, item := range s.ai.Providers() {
			if item.IsDefault {
				providerID = item.ID
				break
			}
		}
	}
	if providerID == "" {
		providerID = "deepseek"
	}

	if hooks.OnGenerateAI != nil {
		hooks.OnGenerateAI()
	}
	response, err := s.ai.Query(ctx, req, rows, newsItems)
	if hooks.OnDelta != nil && response.Answer != "" {
		hooks.OnDelta(response.Answer)
	}
	if err != nil {
		return domain.CopilotQueryResponse{}, "", err
	}

	if hooks.OnSaveSession != nil {
		hooks.OnSaveSession()
	}
	sessionID, sessionDate, err := s.repo.EnsureDailyAISession(ctx, userID, req.Symbol, req.StartDate, req.EndDate)
	if err == nil {
		_ = s.repo.SaveAIMessage(ctx, sessionID, "user", req.Question)
		_ = s.repo.SaveAIMessage(ctx, sessionID, "assistant", response.Answer)
		title, summary := BuildSessionSummary(req.Question, response.Answer)
		_ = s.repo.UpdateAISessionSummary(ctx, sessionID, title, summary)
		response.SessionID = sessionID
		response.SessionDate = sessionDate
	}

	return response, providerID, nil
}

func (s *Service) QueryStreamWithHooks(ctx context.Context, userID string, req domain.CopilotQueryRequest, rows []domain.OHLCRow, hooks QueryHooks) (domain.CopilotQueryResponse, string, error) {
	symbolRecord, _ := s.repo.FindSymbol(ctx, req.Symbol)
	newsKeyword := symbolRecord.Name
	if newsKeyword == "" {
		newsKeyword = req.Symbol
	}
	if hooks.OnFetchNews != nil {
		hooks.OnFetchNews()
	}
	newsItems, _ := s.news.SearchSymbolNews(ctx, newsKeyword, req.StartDate, req.EndDate, 5)

	providerID := strings.TrimSpace(strings.ToLower(req.Provider))
	if providerID == "" {
		for _, item := range s.ai.Providers() {
			if item.IsDefault {
				providerID = item.ID
				break
			}
		}
	}
	if providerID == "" {
		providerID = "deepseek"
	}

	if hooks.OnGenerateAI != nil {
		hooks.OnGenerateAI()
	}
	response, err := s.ai.QueryStream(ctx, req, rows, newsItems, hooks.OnDelta)
	if err != nil {
		return domain.CopilotQueryResponse{}, "", err
	}

	if hooks.OnSaveSession != nil {
		hooks.OnSaveSession()
	}
	sessionID, sessionDate, err := s.repo.EnsureDailyAISession(ctx, userID, req.Symbol, req.StartDate, req.EndDate)
	if err == nil {
		_ = s.repo.SaveAIMessage(ctx, sessionID, "user", req.Question)
		_ = s.repo.SaveAIMessage(ctx, sessionID, "assistant", response.Answer)
		title, summary := BuildSessionSummary(req.Question, response.Answer)
		_ = s.repo.UpdateAISessionSummary(ctx, sessionID, title, summary)
		response.SessionID = sessionID
		response.SessionDate = sessionDate
	}

	return response, providerID, nil
}

func (s *Service) ListSessions(ctx context.Context, userID, symbol string) ([]domain.AISessionSummary, error) {
	return s.repo.ListRecentAISessions(ctx, userID, symbol)
}

func (s *Service) GetSessionMessages(ctx context.Context, userID, sessionID string) (domain.AISessionMessagesResponse, error) {
	return s.repo.GetAISessionMessages(ctx, userID, sessionID)
}

func (s *Service) ToggleSessionFavorite(ctx context.Context, userID, sessionID string, isFavorite bool) error {
	return s.repo.ToggleAISessionFavorite(ctx, userID, sessionID, isFavorite)
}

func (s *Service) CompressOldSessions(ctx context.Context, userID string, daysAgo int) error {
	return s.repo.CompressOldSessions(ctx, userID, daysAgo)
}

func (s *Service) ExpandSession(ctx context.Context, userID, sessionID string) error {
	return s.repo.ExpandSession(ctx, userID, sessionID)
}

func BuildSessionSummary(question, answer string) (string, string) {
	question = strings.TrimSpace(question)
	answer = strings.TrimSpace(answer)

	title := question
	if len([]rune(title)) > 24 {
		title = string([]rune(title)[:24]) + "..."
	}

	summary := answer
	if len([]rune(summary)) > 80 {
		summary = string([]rune(summary)[:80]) + "..."
	}

	return title, summary
}
