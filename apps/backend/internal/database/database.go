package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"market-project/backend/internal/config"
	"market-project/backend/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewPool(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func EnsureSchema(ctx context.Context, pool *pgxpool.Pool) error {
	schema := `
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  is_admin BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS symbols (
  symbol TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  market TEXT NOT NULL,
  source TEXT NOT NULL DEFAULT 'eastmoney',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS symbol_catalog (
  symbol TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  market TEXT NOT NULL,
  source TEXT NOT NULL DEFAULT 'eastmoney',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ohlc_daily (
  symbol TEXT NOT NULL REFERENCES symbols(symbol) ON DELETE CASCADE,
  market TEXT NOT NULL,
  date DATE NOT NULL,
  open DOUBLE PRECISION NOT NULL,
  high DOUBLE PRECISION NOT NULL,
  low DOUBLE PRECISION NOT NULL,
  close DOUBLE PRECISION NOT NULL,
  volume DOUBLE PRECISION NOT NULL,
  amount DOUBLE PRECISION,
  change_rate DOUBLE PRECISION,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (symbol, date)
);

CREATE TABLE IF NOT EXISTS ai_sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  symbol TEXT NOT NULL,
  session_date DATE NOT NULL DEFAULT CURRENT_DATE,
  start_date DATE,
  end_date DATE,
  title TEXT NOT NULL DEFAULT '',
  summary TEXT NOT NULL DEFAULT '',
  is_favorite BOOLEAN NOT NULL DEFAULT FALSE,
  is_compressed BOOLEAN NOT NULL DEFAULT FALSE,
  compressed_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS ai_messages (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  session_id UUID NOT NULL REFERENCES ai_sessions(id) ON DELETE CASCADE,
  role TEXT NOT NULL,
  content TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ohlc_daily_symbol_date ON ohlc_daily(symbol, date DESC);
CREATE INDEX IF NOT EXISTS idx_symbol_catalog_name ON symbol_catalog(name);
CREATE INDEX IF NOT EXISTS idx_symbol_catalog_updated_at ON symbol_catalog(updated_at DESC);

ALTER TABLE ai_sessions ADD COLUMN IF NOT EXISTS session_date DATE;
ALTER TABLE ai_sessions ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE ai_sessions ADD COLUMN IF NOT EXISTS title TEXT NOT NULL DEFAULT '';
ALTER TABLE ai_sessions ADD COLUMN IF NOT EXISTS summary TEXT NOT NULL DEFAULT '';
ALTER TABLE ai_sessions ADD COLUMN IF NOT EXISTS is_favorite BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE ai_sessions ADD COLUMN IF NOT EXISTS is_compressed BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE ai_sessions ADD COLUMN IF NOT EXISTS compressed_at TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_admin BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE ai_sessions SET session_date = COALESCE(session_date, created_at::date, CURRENT_DATE) WHERE session_date IS NULL;
ALTER TABLE ai_sessions ALTER COLUMN session_date SET NOT NULL;

DELETE FROM ai_messages
WHERE session_id IN (
  SELECT id
  FROM (
    SELECT id,
           ROW_NUMBER() OVER (
             PARTITION BY user_id, symbol, session_date
             ORDER BY updated_at DESC, created_at DESC, id DESC
           ) AS rn
    FROM ai_sessions
  ) duplicated
  WHERE rn > 1
);

DELETE FROM ai_sessions
WHERE id IN (
  SELECT id
  FROM (
    SELECT id,
           ROW_NUMBER() OVER (
             PARTITION BY user_id, symbol, session_date
             ORDER BY updated_at DESC, created_at DESC, id DESC
           ) AS rn
    FROM ai_sessions
  ) duplicated
  WHERE rn > 1
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_sessions_user_symbol_day ON ai_sessions(user_id, symbol, session_date);
CREATE INDEX IF NOT EXISTS idx_ai_sessions_user_symbol_recent ON ai_sessions(user_id, symbol, session_date DESC);
`

	_, err := pool.Exec(ctx, schema)
	return err
}

func SeedDemoUser(ctx context.Context, pool *pgxpool.Pool, email, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	_, err = pool.Exec(ctx, `
INSERT INTO users (email, password_hash, is_admin)
VALUES ($1, $2, TRUE)
ON CONFLICT (email) DO UPDATE
SET password_hash = EXCLUDED.password_hash
`, strings.ToLower(strings.TrimSpace(email)), string(hash))
	return err
}

func (r *Repository) CreateUser(ctx context.Context, email, password string) (domain.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, err
	}

	var user domain.User
	err = r.pool.QueryRow(ctx, `
INSERT INTO users (email, password_hash)
VALUES ($1, $2)
RETURNING id::text, email, password_hash
`, strings.ToLower(strings.TrimSpace(email)), string(hash)).Scan(&user.ID, &user.Email, &user.PasswordHash)
	return user, err
}

func (r *Repository) FindUserByEmail(ctx context.Context, email string) (domain.User, error) {
	var user domain.User
	err := r.pool.QueryRow(ctx, `
SELECT id::text, email, password_hash
FROM users
WHERE email = $1
`, strings.ToLower(strings.TrimSpace(email))).Scan(&user.ID, &user.Email, &user.PasswordHash)
	return user, err
}

func (r *Repository) ListSymbols(ctx context.Context) ([]domain.Symbol, error) {
	rows, err := r.pool.Query(ctx, `
SELECT s.symbol,
       COALESCE(NULLIF(sc.name, ''), s.name) AS name,
       COALESCE(NULLIF(sc.market, ''), s.market) AS market,
       s.source
FROM symbols s
LEFT JOIN symbol_catalog sc ON sc.symbol = s.symbol
ORDER BY symbol
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var symbols []domain.Symbol
	for rows.Next() {
		var symbol domain.Symbol
		if err := rows.Scan(&symbol.Symbol, &symbol.Name, &symbol.Market, &symbol.Source); err != nil {
			return nil, err
		}
		symbols = append(symbols, symbol)
	}

	return symbols, rows.Err()
}

func (r *Repository) FindSymbolCatalogByCode(ctx context.Context, symbolCode string) (domain.Symbol, error) {
	var symbol domain.Symbol
	err := r.pool.QueryRow(ctx, `
SELECT symbol, name, market, source
FROM symbol_catalog
WHERE symbol = $1
`, strings.TrimSpace(symbolCode)).Scan(&symbol.Symbol, &symbol.Name, &symbol.Market, &symbol.Source)
	return symbol, err
}

func (r *Repository) SearchSymbolCatalog(ctx context.Context, query string, limit int) ([]domain.Symbol, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []domain.Symbol{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	pattern := "%" + query + "%"
	prefix := query + "%"

	rows, err := r.pool.Query(ctx, `
SELECT symbol, name, market, source
FROM symbol_catalog
WHERE symbol ILIKE $1 OR name ILIKE $2
ORDER BY
  CASE
    WHEN symbol = $3 THEN 0
    WHEN symbol ILIKE $4 THEN 1
    WHEN name ILIKE $2 THEN 2
    ELSE 3
  END,
  symbol
LIMIT $5
`, pattern, pattern, query, prefix, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var symbols []domain.Symbol
	for rows.Next() {
		var symbol domain.Symbol
		if err := rows.Scan(&symbol.Symbol, &symbol.Name, &symbol.Market, &symbol.Source); err != nil {
			return nil, err
		}
		symbols = append(symbols, symbol)
	}

	return symbols, rows.Err()
}

func (r *Repository) UpsertSymbolCatalog(ctx context.Context, symbols []domain.Symbol) error {
	if len(symbols) == 0 {
		return nil
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, symbol := range symbols {
		_, err = tx.Exec(ctx, `
INSERT INTO symbol_catalog (symbol, name, market, source, updated_at)
VALUES ($1, $2, $3, $4, NOW())
ON CONFLICT (symbol) DO UPDATE
SET name = EXCLUDED.name,
    market = EXCLUDED.market,
    source = EXCLUDED.source,
    updated_at = NOW()
`, symbol.Symbol, symbol.Name, symbol.Market, symbol.Source)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *Repository) CountSymbolCatalog(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM symbol_catalog
`).Scan(&count)
	return count, err
}

func (r *Repository) FindSymbol(ctx context.Context, symbolCode string) (domain.Symbol, error) {
	var symbol domain.Symbol
	err := r.pool.QueryRow(ctx, `
SELECT symbol, name, market, source
FROM symbols
WHERE symbol = $1
`, strings.TrimSpace(symbolCode)).Scan(&symbol.Symbol, &symbol.Name, &symbol.Market, &symbol.Source)
	return symbol, err
}

func (r *Repository) DeleteSymbol(ctx context.Context, symbolCode string) error {
	_, err := r.pool.Exec(ctx, `
DELETE FROM symbols
WHERE symbol = $1
`, strings.TrimSpace(symbolCode))
	return err
}

func (r *Repository) UpsertSymbol(ctx context.Context, symbol domain.Symbol) error {
	_, err := r.pool.Exec(ctx, `
INSERT INTO symbols (symbol, name, market, source)
VALUES ($1, $2, $3, $4)
ON CONFLICT (symbol) DO UPDATE
SET name = EXCLUDED.name,
    market = EXCLUDED.market,
    source = EXCLUDED.source
`, symbol.Symbol, symbol.Name, symbol.Market, symbol.Source)
	return err
}

func (r *Repository) UpsertSymbolWithRows(ctx context.Context, symbol domain.Symbol, rows []domain.OHLCRow) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
INSERT INTO symbols (symbol, name, market, source)
VALUES ($1, $2, $3, $4)
ON CONFLICT (symbol) DO UPDATE
SET name = EXCLUDED.name,
    market = EXCLUDED.market,
    source = EXCLUDED.source
`, symbol.Symbol, symbol.Name, symbol.Market, symbol.Source)
	if err != nil {
		return err
	}

	for _, row := range rows {
		_, err = tx.Exec(ctx, `
INSERT INTO ohlc_daily (symbol, market, date, open, high, low, close, volume, amount, change_rate)
VALUES ($1, $2, $3::date, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (symbol, date) DO UPDATE
SET market = EXCLUDED.market,
    open = EXCLUDED.open,
    high = EXCLUDED.high,
    low = EXCLUDED.low,
    close = EXCLUDED.close,
    volume = EXCLUDED.volume,
    amount = EXCLUDED.amount,
    change_rate = EXCLUDED.change_rate
`, row.Symbol, row.Market, row.Date, row.Open, row.High, row.Low, row.Close, row.Volume, row.Amount, row.ChangeRate)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *Repository) ListOHLC(ctx context.Context, symbol, startDate, endDate string) ([]domain.OHLCRow, error) {
	query := `
SELECT symbol, market, date::text, open, high, low, close, volume, amount, change_rate
FROM ohlc_daily
WHERE symbol = $1
`
	args := []any{symbol}

	if startDate != "" {
		query += fmt.Sprintf(" AND date >= $%d::date", len(args)+1)
		args = append(args, startDate)
	}
	if endDate != "" {
		query += fmt.Sprintf(" AND date <= $%d::date", len(args)+1)
		args = append(args, endDate)
	}

	query += " ORDER BY date"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]domain.OHLCRow, 0)
	for rows.Next() {
		var row domain.OHLCRow
		if err := rows.Scan(
			&row.Symbol,
			&row.Market,
			&row.Date,
			&row.Open,
			&row.High,
			&row.Low,
			&row.Close,
			&row.Volume,
			&row.Amount,
			&row.ChangeRate,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}

	return result, rows.Err()
}

func (r *Repository) LatestOHLCDate(ctx context.Context, symbol string) (string, error) {
	var latest string
	err := r.pool.QueryRow(ctx, `
SELECT COALESCE(MAX(date)::text, '')
FROM ohlc_daily
WHERE symbol = $1
`, symbol).Scan(&latest)
	return latest, err
}

func (r *Repository) EnsureDailyAISession(ctx context.Context, userID, symbol, startDate, endDate string) (string, string, error) {
	sessionDate := time.Now().Format("2006-01-02")

	_, err := r.pool.Exec(ctx, `
DELETE FROM ai_sessions
WHERE user_id = $1::uuid
  AND symbol = $2
  AND session_date < CURRENT_DATE - INTERVAL '6 days'
`, userID, symbol)
	if err != nil {
		return "", "", err
	}

	var sessionID string
	err = r.pool.QueryRow(ctx, `
INSERT INTO ai_sessions (id, user_id, symbol, session_date, start_date, end_date, updated_at)
VALUES ($1::uuid, $2::uuid, $3, CURRENT_DATE, NULLIF($4, '')::date, NULLIF($5, '')::date, NOW())
ON CONFLICT (user_id, symbol, session_date) DO UPDATE
SET start_date = COALESCE(EXCLUDED.start_date, ai_sessions.start_date),
    end_date = COALESCE(EXCLUDED.end_date, ai_sessions.end_date),
    updated_at = NOW()
RETURNING id::text, session_date::text
`, uuid.NewString(), userID, symbol, startDate, endDate).Scan(&sessionID, &sessionDate)
	return sessionID, sessionDate, err
}

func (r *Repository) SaveAIMessage(ctx context.Context, sessionID, role, content string) error {
	_, err := r.pool.Exec(ctx, `
INSERT INTO ai_messages (id, session_id, role, content)
VALUES ($1::uuid, $2::uuid, $3, $4)
`, uuid.NewString(), sessionID, role, content)
	if err != nil {
		return err
	}

	_, err = r.pool.Exec(ctx, `
UPDATE ai_sessions
SET updated_at = NOW()
WHERE id = $1::uuid
`, sessionID)
	return err
}

func (r *Repository) UpdateAISessionSummary(ctx context.Context, sessionID, title, summary string) error {
	_, err := r.pool.Exec(ctx, `
UPDATE ai_sessions
SET title = $2,
    summary = $3,
    updated_at = NOW()
WHERE id = $1::uuid
`, sessionID, title, summary)
	return err
}

func (r *Repository) ListRecentAISessions(ctx context.Context, userID, symbol string) ([]domain.AISessionSummary, error) {
	rows, err := r.pool.Query(ctx, `
SELECT
  s.id::text,
  s.session_date::text,
  s.symbol,
  COUNT(m.id)::int AS message_count,
  s.updated_at::text,
  s.title,
  s.summary,
  s.is_favorite,
  s.is_compressed
FROM ai_sessions s
LEFT JOIN ai_messages m ON m.session_id = s.id
WHERE s.user_id = $1::uuid
  AND s.symbol = $2
  AND s.session_date >= CURRENT_DATE - INTERVAL '6 days'
GROUP BY s.id, s.session_date, s.symbol, s.updated_at, s.title, s.summary, s.is_favorite, s.is_compressed
ORDER BY s.is_favorite DESC, s.session_date DESC, s.updated_at DESC
`, userID, symbol)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []domain.AISessionSummary
	for rows.Next() {
		var session domain.AISessionSummary
		if err := rows.Scan(
			&session.ID,
			&session.SessionDate,
			&session.Symbol,
			&session.MessageCount,
			&session.UpdatedAt,
			&session.Title,
			&session.Summary,
			&session.IsFavorite,
			&session.IsCompressed,
		); err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}

	return sessions, rows.Err()
}

func (r *Repository) GetAISessionMessages(ctx context.Context, userID, sessionID string) (domain.AISessionMessagesResponse, error) {
	var response domain.AISessionMessagesResponse
	err := r.pool.QueryRow(ctx, `
SELECT id::text, session_date::text, symbol, updated_at::text, title, summary, is_favorite
FROM ai_sessions
WHERE id = $1::uuid AND user_id = $2::uuid
`, sessionID, userID).Scan(
		&response.Session.ID,
		&response.Session.SessionDate,
		&response.Session.Symbol,
		&response.Session.UpdatedAt,
		&response.Session.Title,
		&response.Session.Summary,
		&response.Session.IsFavorite,
	)
	if err != nil {
		return response, err
	}

	rows, err := r.pool.Query(ctx, `
SELECT role, content
FROM ai_messages
WHERE session_id = $1::uuid
ORDER BY created_at, id
`, sessionID)
	if err != nil {
		return response, err
	}
	defer rows.Close()

	for rows.Next() {
		var message domain.ChatMessage
		if err := rows.Scan(&message.Role, &message.Content); err != nil {
			return response, err
		}
		response.Messages = append(response.Messages, message)
	}
	response.Session.MessageCount = len(response.Messages)

	return response, rows.Err()
}

func (r *Repository) ToggleAISessionFavorite(ctx context.Context, userID, sessionID string, isFavorite bool) error {
	_, err := r.pool.Exec(ctx, `
UPDATE ai_sessions
SET is_favorite = $3,
    updated_at = NOW()
WHERE id = $1::uuid
  AND user_id = $2::uuid
`, sessionID, userID, isFavorite)
	return err
}

func (r *Repository) CompressOldSessions(ctx context.Context, userID string, daysAgo int) error {
	_, err := r.pool.Exec(ctx, `
UPDATE ai_sessions
SET is_compressed = TRUE,
    compressed_at = NOW(),
    updated_at = NOW()
WHERE user_id = $1::uuid
  AND session_date < CURRENT_DATE - INTERVAL $2 || ' days'
  AND is_compressed = FALSE
`, userID, daysAgo)
	return err
}

func (r *Repository) ExpandSession(ctx context.Context, userID, sessionID string) error {
	_, err := r.pool.Exec(ctx, `
UPDATE ai_sessions
SET is_compressed = FALSE,
    compressed_at = NULL,
    updated_at = NOW()
WHERE id = $1::uuid
  AND user_id = $2::uuid
`, sessionID, userID)
	return err
}
