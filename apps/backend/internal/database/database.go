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
  credit_balance INTEGER NOT NULL DEFAULT 0,
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

CREATE TABLE IF NOT EXISTS recharge_orders (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  package_id TEXT NOT NULL,
  package_name TEXT NOT NULL DEFAULT '',
  payment_method TEXT NOT NULL,
  amount_cny INTEGER NOT NULL,
  credits INTEGER NOT NULL,
  daily_quota INTEGER NOT NULL DEFAULT 0,
  duration_days INTEGER NOT NULL DEFAULT 30,
  payment_url TEXT NOT NULL DEFAULT '',
  qr_code TEXT NOT NULL DEFAULT '',
  provider_order_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending',
  paid_at TIMESTAMPTZ,
  activated_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS usage_records (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  symbol TEXT NOT NULL,
  cost_credits INTEGER NOT NULL,
  membership_quota_used INTEGER NOT NULL DEFAULT 0,
  bonus_credits_used INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_memberships (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  package_id TEXT NOT NULL,
  package_name TEXT NOT NULL,
  daily_quota INTEGER NOT NULL,
  duration_days INTEGER NOT NULL DEFAULT 30,
  status TEXT NOT NULL DEFAULT 'active',
  starts_at TIMESTAMPTZ NOT NULL,
  ends_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS redeem_codes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code TEXT NOT NULL UNIQUE,
  reward_type TEXT NOT NULL DEFAULT 'bonus_credits',
  bonus_credits INTEGER NOT NULL,
  package_id TEXT NOT NULL DEFAULT '',
  package_name TEXT NOT NULL DEFAULT '',
  daily_quota INTEGER NOT NULL DEFAULT 0,
  duration_days INTEGER NOT NULL DEFAULT 30,
  max_claims INTEGER NOT NULL DEFAULT 1,
  claimed_count INTEGER NOT NULL DEFAULT 0,
  expires_at TIMESTAMPTZ,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS redeem_code_claims (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  redeem_code_id UUID NOT NULL REFERENCES redeem_codes(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  bonus_credits INTEGER NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (redeem_code_id, user_id)
);

CREATE TABLE IF NOT EXISTS admin_action_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  admin_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  admin_email TEXT NOT NULL,
  action_type TEXT NOT NULL,
  target_type TEXT NOT NULL,
  target_id TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ohlc_daily_symbol_date ON ohlc_daily(symbol, date DESC);
CREATE INDEX IF NOT EXISTS idx_symbol_catalog_name ON symbol_catalog(name);
CREATE INDEX IF NOT EXISTS idx_symbol_catalog_updated_at ON symbol_catalog(updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_user_memberships_user_window ON user_memberships(user_id, starts_at DESC, ends_at DESC);
ALTER TABLE ai_sessions ADD COLUMN IF NOT EXISTS session_date DATE;
ALTER TABLE ai_sessions ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE ai_sessions ADD COLUMN IF NOT EXISTS title TEXT NOT NULL DEFAULT '';
ALTER TABLE ai_sessions ADD COLUMN IF NOT EXISTS summary TEXT NOT NULL DEFAULT '';
ALTER TABLE ai_sessions ADD COLUMN IF NOT EXISTS is_favorite BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS credit_balance INTEGER NOT NULL DEFAULT 0;
ALTER TABLE recharge_orders ADD COLUMN IF NOT EXISTS daily_quota INTEGER NOT NULL DEFAULT 0;
ALTER TABLE recharge_orders ADD COLUMN IF NOT EXISTS duration_days INTEGER NOT NULL DEFAULT 30;
ALTER TABLE recharge_orders ADD COLUMN IF NOT EXISTS package_name TEXT NOT NULL DEFAULT '';
ALTER TABLE recharge_orders ADD COLUMN IF NOT EXISTS payment_url TEXT NOT NULL DEFAULT '';
ALTER TABLE recharge_orders ADD COLUMN IF NOT EXISTS qr_code TEXT NOT NULL DEFAULT '';
ALTER TABLE recharge_orders ADD COLUMN IF NOT EXISTS provider_order_id TEXT NOT NULL DEFAULT '';
ALTER TABLE recharge_orders ADD COLUMN IF NOT EXISTS paid_at TIMESTAMPTZ;
ALTER TABLE recharge_orders ADD COLUMN IF NOT EXISTS activated_at TIMESTAMPTZ;
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS membership_quota_used INTEGER NOT NULL DEFAULT 0;
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS bonus_credits_used INTEGER NOT NULL DEFAULT 0;
ALTER TABLE redeem_codes ADD COLUMN IF NOT EXISTS reward_type TEXT NOT NULL DEFAULT 'bonus_credits';
ALTER TABLE redeem_codes ADD COLUMN IF NOT EXISTS package_id TEXT NOT NULL DEFAULT '';
ALTER TABLE redeem_codes ADD COLUMN IF NOT EXISTS package_name TEXT NOT NULL DEFAULT '';
ALTER TABLE redeem_codes ADD COLUMN IF NOT EXISTS daily_quota INTEGER NOT NULL DEFAULT 0;
ALTER TABLE redeem_codes ADD COLUMN IF NOT EXISTS duration_days INTEGER NOT NULL DEFAULT 30;
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
CREATE INDEX IF NOT EXISTS idx_admin_action_logs_created_at ON admin_action_logs(created_at DESC);
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
INSERT INTO users (email, password_hash)
VALUES ($1, $2)
ON CONFLICT (email) DO UPDATE
SET password_hash = EXCLUDED.password_hash
`, strings.ToLower(strings.TrimSpace(email)), string(hash))
	return err
}

func SeedRedeemCode(ctx context.Context, pool *pgxpool.Pool, code string, bonusCredits, maxClaims int) error {
	_, err := pool.Exec(ctx, `
INSERT INTO redeem_codes (code, reward_type, bonus_credits, max_claims, is_active)
VALUES ($1, 'bonus_credits', $2, $3, TRUE)
ON CONFLICT (code) DO UPDATE
SET reward_type = EXCLUDED.reward_type,
    bonus_credits = EXCLUDED.bonus_credits,
    max_claims = EXCLUDED.max_claims,
    is_active = TRUE
`, strings.TrimSpace(strings.ToUpper(code)), bonusCredits, maxClaims)
	return err
}

func SeedMembershipRedeemCode(ctx context.Context, pool *pgxpool.Pool, code, packageID, packageName string, dailyQuota, durationDays, maxClaims int) error {
	_, err := pool.Exec(ctx, `
INSERT INTO redeem_codes (code, reward_type, bonus_credits, package_id, package_name, daily_quota, duration_days, max_claims, is_active)
VALUES ($1, 'membership', 0, $2, $3, $4, $5, $6, TRUE)
ON CONFLICT (code) DO UPDATE
SET reward_type = EXCLUDED.reward_type,
    package_id = EXCLUDED.package_id,
    package_name = EXCLUDED.package_name,
    daily_quota = EXCLUDED.daily_quota,
    duration_days = EXCLUDED.duration_days,
    max_claims = EXCLUDED.max_claims,
    is_active = TRUE
`, strings.TrimSpace(strings.ToUpper(code)), packageID, packageName, dailyQuota, durationDays, maxClaims)
	return err
}

func (r *Repository) CreateAdminRedeemCode(ctx context.Context, payload domain.AdminRedeemCode) (domain.AdminRedeemCode, error) {
	var item domain.AdminRedeemCode
	err := r.pool.QueryRow(ctx, `
INSERT INTO redeem_codes (code, reward_type, bonus_credits, package_id, package_name, daily_quota, duration_days, max_claims, expires_at, is_active)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, '')::timestamptz, TRUE)
RETURNING code, reward_type, bonus_credits, package_id, package_name, daily_quota, duration_days, max_claims, claimed_count, is_active, COALESCE(expires_at::text, ''), created_at::text
`,
		strings.TrimSpace(strings.ToUpper(payload.Code)),
		payload.RewardType,
		payload.BonusCredits,
		payload.PackageID,
		payload.PackageName,
		payload.DailyQuota,
		payload.DurationDays,
		payload.MaxClaims,
		payload.ExpiresAt,
	).Scan(
		&item.Code,
		&item.RewardType,
		&item.BonusCredits,
		&item.PackageID,
		&item.PackageName,
		&item.DailyQuota,
		&item.DurationDays,
		&item.MaxClaims,
		&item.ClaimedCount,
		&item.IsActive,
		&item.ExpiresAt,
		&item.CreatedAt,
	)
	return item, err
}

func (r *Repository) ListAdminRedeemCodes(ctx context.Context, search, rewardType, status string) ([]domain.AdminRedeemCode, error) {
	search = strings.TrimSpace(strings.ToUpper(search))
	rewardType = strings.TrimSpace(strings.ToLower(rewardType))
	status = strings.TrimSpace(strings.ToLower(status))

	rows, err := r.pool.Query(ctx, `
SELECT code, reward_type, bonus_credits, package_id, package_name, daily_quota, duration_days, max_claims, claimed_count, is_active, COALESCE(expires_at::text, ''), created_at::text
FROM redeem_codes
WHERE ($1 = '' OR code LIKE '%' || $1 || '%')
  AND ($2 = '' OR reward_type = $2)
  AND (
    $3 = ''
    OR ($3 = 'active' AND is_active = TRUE)
    OR ($3 = 'disabled' AND is_active = FALSE)
  )
ORDER BY created_at DESC
LIMIT 50
`, search, rewardType, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.AdminRedeemCode
	for rows.Next() {
		var item domain.AdminRedeemCode
		if err := rows.Scan(
			&item.Code,
			&item.RewardType,
			&item.BonusCredits,
			&item.PackageID,
			&item.PackageName,
			&item.DailyQuota,
			&item.DurationDays,
			&item.MaxClaims,
			&item.ClaimedCount,
			&item.IsActive,
			&item.ExpiresAt,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) DisableAdminRedeemCode(ctx context.Context, code string) error {
	tag, err := r.pool.Exec(ctx, `
UPDATE redeem_codes
SET is_active = FALSE
WHERE code = $1
`, strings.TrimSpace(strings.ToUpper(code)))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *Repository) ListAdminRedeemCodeClaims(ctx context.Context, search string) ([]domain.AdminRedeemCodeClaim, error) {
	search = strings.TrimSpace(strings.ToUpper(search))

	rows, err := r.pool.Query(ctx, `
SELECT rc.code, rc.reward_type, u.email, rcc.bonus_credits, rc.package_name, rcc.created_at::text
FROM redeem_code_claims rcc
JOIN redeem_codes rc ON rc.id = rcc.redeem_code_id
JOIN users u ON u.id = rcc.user_id
WHERE ($1 = '' OR rc.code LIKE '%' || $1 || '%' OR UPPER(u.email) LIKE '%' || $1 || '%')
ORDER BY rcc.created_at DESC
LIMIT 50
`, search)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.AdminRedeemCodeClaim
	for rows.Next() {
		var item domain.AdminRedeemCodeClaim
		if err := rows.Scan(
			&item.Code,
			&item.RewardType,
			&item.UserEmail,
			&item.BonusCredits,
			&item.PackageName,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) ListAdminUsers(ctx context.Context, adminEmails []string, search, membershipStatus string) ([]domain.AdminUserSummary, error) {
	search = strings.TrimSpace(strings.ToLower(search))
	membershipStatus = strings.TrimSpace(strings.ToLower(membershipStatus))

	rows, err := r.pool.Query(ctx, `
SELECT
  u.id::text,
  u.email,
  u.credit_balance,
  COALESCE(m.package_name, ''),
  COALESCE(m.daily_quota, 0),
  COALESCE(m.ends_at::text, ''),
  u.created_at::text
FROM users u
LEFT JOIN LATERAL (
  SELECT package_name, daily_quota, ends_at
  FROM user_memberships
  WHERE user_id = u.id
    AND status = 'active'
    AND starts_at <= NOW()
    AND ends_at > NOW()
  ORDER BY ends_at DESC, created_at DESC
  LIMIT 1
) m ON TRUE
WHERE ($1 = '' OR LOWER(u.email) LIKE '%' || $1 || '%')
  AND (
    $2 = ''
    OR ($2 = 'member' AND COALESCE(m.package_name, '') <> '')
    OR ($2 = 'non_member' AND COALESCE(m.package_name, '') = '')
  )
ORDER BY u.created_at DESC
LIMIT 50
`, search, membershipStatus)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	adminSet := make(map[string]struct{}, len(adminEmails))
	for _, email := range adminEmails {
		adminSet[strings.TrimSpace(strings.ToLower(email))] = struct{}{}
	}

	var result []domain.AdminUserSummary
	for rows.Next() {
		var item domain.AdminUserSummary
		if err := rows.Scan(
			&item.UserID,
			&item.Email,
			&item.CreditBalance,
			&item.CurrentPackage,
			&item.DailyQuota,
			&item.MembershipEndsAt,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		_, item.IsAdmin = adminSet[strings.TrimSpace(strings.ToLower(item.Email))]
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) GetAdminUserSummary(ctx context.Context, adminEmails []string, userID string) (domain.AdminUserSummary, error) {
	var item domain.AdminUserSummary
	err := r.pool.QueryRow(ctx, `
SELECT
  u.id::text,
  u.email,
  u.credit_balance,
  COALESCE(m.package_name, ''),
  COALESCE(m.daily_quota, 0),
  COALESCE(m.ends_at::text, ''),
  u.created_at::text
FROM users u
LEFT JOIN LATERAL (
  SELECT package_name, daily_quota, ends_at
  FROM user_memberships
  WHERE user_id = u.id
    AND status = 'active'
    AND starts_at <= NOW()
    AND ends_at > NOW()
  ORDER BY ends_at DESC, created_at DESC
  LIMIT 1
) m ON TRUE
WHERE u.id = $1::uuid
`, userID).Scan(
		&item.UserID,
		&item.Email,
		&item.CreditBalance,
		&item.CurrentPackage,
		&item.DailyQuota,
		&item.MembershipEndsAt,
		&item.CreatedAt,
	)
	if err != nil {
		return domain.AdminUserSummary{}, err
	}

	adminSet := make(map[string]struct{}, len(adminEmails))
	for _, email := range adminEmails {
		adminSet[strings.TrimSpace(strings.ToLower(email))] = struct{}{}
	}
	_, item.IsAdmin = adminSet[strings.TrimSpace(strings.ToLower(item.Email))]
	return item, nil
}

func (r *Repository) ListAdminUserMemberships(ctx context.Context, userID string) ([]domain.AdminUserMembershipRecord, error) {
	rows, err := r.pool.Query(ctx, `
SELECT id::text, package_id, package_name, status, daily_quota, duration_days, starts_at::text, ends_at::text, created_at::text
FROM user_memberships
WHERE user_id = $1::uuid
ORDER BY starts_at DESC, created_at DESC
LIMIT 10
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.AdminUserMembershipRecord
	for rows.Next() {
		var item domain.AdminUserMembershipRecord
		if err := rows.Scan(
			&item.ID,
			&item.PackageID,
			&item.PackageName,
			&item.Status,
			&item.DailyQuota,
			&item.DurationDays,
			&item.StartsAt,
			&item.EndsAt,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) ListAdminUserRedeemClaims(ctx context.Context, userID string) ([]domain.AdminUserRedeemClaim, error) {
	rows, err := r.pool.Query(ctx, `
SELECT rc.code, rc.reward_type, rcc.bonus_credits, rc.package_name, rcc.created_at::text
FROM redeem_code_claims rcc
JOIN redeem_codes rc ON rc.id = rcc.redeem_code_id
WHERE rcc.user_id = $1::uuid
ORDER BY rcc.created_at DESC
LIMIT 10
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.AdminUserRedeemClaim
	for rows.Next() {
		var item domain.AdminUserRedeemClaim
		if err := rows.Scan(
			&item.Code,
			&item.RewardType,
			&item.BonusCredits,
			&item.PackageName,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) LogAdminAction(ctx context.Context, adminUserID, adminEmail, actionType, targetType, targetID, description string) error {
	_, err := r.pool.Exec(ctx, `
INSERT INTO admin_action_logs (id, admin_user_id, admin_email, action_type, target_type, target_id, description)
VALUES ($1::uuid, NULLIF($2, '')::uuid, $3, $4, $5, $6, $7)
`, uuid.NewString(), adminUserID, strings.TrimSpace(strings.ToLower(adminEmail)), actionType, targetType, targetID, description)
	return err
}

func (r *Repository) ListAdminActionLogs(ctx context.Context, search string) ([]domain.AdminActionLog, error) {
	search = strings.TrimSpace(strings.ToLower(search))

	rows, err := r.pool.Query(ctx, `
SELECT id::text, admin_email, action_type, target_type, target_id, description, created_at::text
FROM admin_action_logs
WHERE ($1 = ''
  OR LOWER(admin_email) LIKE '%' || $1 || '%'
  OR LOWER(action_type) LIKE '%' || $1 || '%'
  OR LOWER(target_type) LIKE '%' || $1 || '%'
  OR LOWER(target_id) LIKE '%' || $1 || '%'
  OR LOWER(description) LIKE '%' || $1 || '%')
ORDER BY created_at DESC
LIMIT 50
`, search)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.AdminActionLog
	for rows.Next() {
		var item domain.AdminActionLog
		if err := rows.Scan(
			&item.ID,
			&item.AdminEmail,
			&item.ActionType,
			&item.TargetType,
			&item.TargetID,
			&item.Description,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) AddAdminBonusCredits(ctx context.Context, userID string, amount int) (int, error) {
	var balance int
	err := r.pool.QueryRow(ctx, `
UPDATE users
SET credit_balance = credit_balance + $2
WHERE id = $1::uuid
RETURNING credit_balance
`, userID, amount).Scan(&balance)
	return balance, err
}

func (r *Repository) GrantAdminMembership(ctx context.Context, userID string, pkg domain.BillingPackage) (*domain.MembershipStatus, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	membership, err := r.activateMembershipTx(ctx, tx, userID, pkg.ID, pkg.Name, pkg.DailyQuota, pkg.DurationDays)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return membership, nil
}

func (r *Repository) CreateUser(ctx context.Context, email, password string) (domain.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, err
	}

	var user domain.User
	err = r.pool.QueryRow(ctx, `
INSERT INTO users (email, password_hash, credit_balance)
VALUES ($1, $2, 50000)
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
  s.is_favorite
FROM ai_sessions s
LEFT JOIN ai_messages m ON m.session_id = s.id
WHERE s.user_id = $1::uuid
  AND s.symbol = $2
  AND s.session_date >= CURRENT_DATE - INTERVAL '6 days'
GROUP BY s.id, s.session_date, s.symbol, s.updated_at, s.title, s.summary, s.is_favorite
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

func (r *Repository) GetUserCreditBalance(ctx context.Context, userID string) (int, error) {
	var balance int
	err := r.pool.QueryRow(ctx, `
SELECT credit_balance
FROM users
WHERE id = $1::uuid
`, userID).Scan(&balance)
	return balance, err
}

func (r *Repository) GetAIAllowanceStatus(ctx context.Context, userID string) (domain.AIAllowanceStatus, error) {
	balance, err := r.GetUserCreditBalance(ctx, userID)
	if err != nil {
		return domain.AIAllowanceStatus{}, err
	}

	membership, err := r.GetCurrentMembership(ctx, userID)
	if err != nil {
		return domain.AIAllowanceStatus{}, err
	}

	todayQuota := domain.DailyQuotaStatus{
		Date:      time.Now().Format("2006-01-02"),
		Total:     0,
		Used:      0,
		Remaining: 0,
	}
	if membership != nil {
		used, err := r.GetTodayMembershipQuotaUsage(ctx, userID)
		if err != nil {
			return domain.AIAllowanceStatus{}, err
		}
		remaining := membership.DailyQuota - used
		if remaining < 0 {
			remaining = 0
		}
		todayQuota = domain.DailyQuotaStatus{
			Date:      time.Now().Format("2006-01-02"),
			Total:     membership.DailyQuota,
			Used:      used,
			Remaining: remaining,
		}
	}

	return domain.AIAllowanceStatus{
		CreditBalance:      balance,
		CurrentMembership:  membership,
		TodayQuota:         todayQuota,
		AvailableToConsume: balance + todayQuota.Remaining,
	}, nil
}

func (r *Repository) GetCurrentMembership(ctx context.Context, userID string) (*domain.MembershipStatus, error) {
	var membership domain.MembershipStatus
	err := r.pool.QueryRow(ctx, `
SELECT package_id, package_name, status, daily_quota, duration_days, starts_at::text, ends_at::text
FROM user_memberships
WHERE user_id = $1::uuid
  AND status = 'active'
  AND starts_at <= NOW()
  AND ends_at > NOW()
ORDER BY ends_at DESC, created_at DESC
LIMIT 1
`, userID).Scan(
		&membership.PackageID,
		&membership.PackageName,
		&membership.Status,
		&membership.DailyQuota,
		&membership.DurationDays,
		&membership.StartsAt,
		&membership.EndsAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &membership, nil
}

func (r *Repository) GetTodayMembershipQuotaUsage(ctx context.Context, userID string) (int, error) {
	var used int
	err := r.pool.QueryRow(ctx, `
SELECT COALESCE(SUM(membership_quota_used), 0)
FROM usage_records
WHERE user_id = $1::uuid
  AND created_at::date = CURRENT_DATE
`, userID).Scan(&used)
	return used, err
}

func (r *Repository) getUserCreditBalanceTx(ctx context.Context, tx pgx.Tx, userID string) (int, error) {
	var balance int
	err := tx.QueryRow(ctx, `
SELECT credit_balance
FROM users
WHERE id = $1::uuid
`, userID).Scan(&balance)
	return balance, err
}

func (r *Repository) activateMembershipTx(ctx context.Context, tx pgx.Tx, userID, packageID, packageName string, dailyQuota, durationDays int) (*domain.MembershipStatus, error) {
	var latestEnd *time.Time
	if err := tx.QueryRow(ctx, `
SELECT ends_at
FROM user_memberships
WHERE user_id = $1::uuid
  AND status = 'active'
  AND ends_at > NOW()
ORDER BY ends_at DESC
LIMIT 1
`, userID).Scan(&latestEnd); err != nil && err != pgx.ErrNoRows {
		return nil, err
	}

	startsAt := time.Now().UTC()
	if latestEnd != nil && latestEnd.After(startsAt) {
		startsAt = *latestEnd
	}
	endsAt := startsAt.Add(time.Duration(durationDays) * 24 * time.Hour)

	if _, err := tx.Exec(ctx, `
INSERT INTO user_memberships (id, user_id, package_id, package_name, daily_quota, duration_days, status, starts_at, ends_at)
VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, 'active', $7, $8)
`, uuid.NewString(), userID, packageID, packageName, dailyQuota, durationDays, startsAt, endsAt); err != nil {
		return nil, err
	}

	return &domain.MembershipStatus{
		PackageID:    packageID,
		PackageName:  packageName,
		Status:       "active",
		DailyQuota:   dailyQuota,
		DurationDays: durationDays,
		StartsAt:     startsAt.Format(time.RFC3339),
		EndsAt:       endsAt.Format(time.RFC3339),
	}, nil
}

func (r *Repository) CreateRechargeOrder(ctx context.Context, userID string, pkg domain.BillingPackage, paymentMethod, paymentURL, qrCode string) (domain.RechargeOrderResponse, error) {
	var order domain.RechargeOrderResponse
	err := r.pool.QueryRow(ctx, `
INSERT INTO recharge_orders (user_id, package_id, package_name, payment_method, amount_cny, credits, daily_quota, duration_days, payment_url, qr_code, status)
VALUES ($1::uuid, $2, $3, $4, $5, 0, $6, $7, $8, $9, 'pending')
RETURNING id::text, status, package_id, payment_method, amount_cny, daily_quota, duration_days, payment_url, qr_code
`, userID, pkg.ID, pkg.Name, paymentMethod, pkg.AmountCNY, pkg.DailyQuota, pkg.DurationDays, paymentURL, qrCode).Scan(
		&order.OrderID,
		&order.Status,
		&order.PackageID,
		&order.PaymentMethod,
		&order.AmountCNY,
		&order.DailyQuota,
		&order.DurationDays,
		&order.PaymentURL,
		&order.QRCode,
	)
	if err != nil {
		return domain.RechargeOrderResponse{}, err
	}
	order.MockPayReady = paymentMethod == "alipay"
	return order, nil
}

func (r *Repository) GetRechargeOrder(ctx context.Context, userID, orderID string) (domain.RechargeOrderResponse, error) {
	var order domain.RechargeOrderResponse
	err := r.pool.QueryRow(ctx, `
SELECT id::text, status, package_id, payment_method, amount_cny, daily_quota, duration_days, payment_url, qr_code
FROM recharge_orders
WHERE id = $1::uuid
  AND user_id = $2::uuid
`, orderID, userID).Scan(
		&order.OrderID,
		&order.Status,
		&order.PackageID,
		&order.PaymentMethod,
		&order.AmountCNY,
		&order.DailyQuota,
		&order.DurationDays,
		&order.PaymentURL,
		&order.QRCode,
	)
	if err != nil {
		return domain.RechargeOrderResponse{}, err
	}
	order.MockPayReady = order.PaymentMethod == "alipay" && order.Status == "pending"
	return order, nil
}

func (r *Repository) MarkRechargeOrderPaid(ctx context.Context, orderID, providerOrderID string) (string, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var userID string
	var status string
	var packageID string
	var packageName string
	var dailyQuota int
	var durationDays int
	err = tx.QueryRow(ctx, `
SELECT user_id::text, status, package_id, package_name, daily_quota, duration_days
FROM recharge_orders
WHERE id = $1::uuid
FOR UPDATE
`, orderID).Scan(&userID, &status, &packageID, &packageName, &dailyQuota, &durationDays)
	if err != nil {
		return "", err
	}

	if status == "paid" {
		if err := tx.Commit(ctx); err != nil {
			return "", err
		}
		return userID, nil
	}

	if _, err := tx.Exec(ctx, `
UPDATE recharge_orders
SET status = 'paid',
    provider_order_id = $2,
    paid_at = NOW(),
    activated_at = NOW()
WHERE id = $1::uuid
`, orderID, providerOrderID); err != nil {
		return "", err
	}

	if _, err := r.activateMembershipTx(ctx, tx, userID, packageID, packageName, dailyQuota, durationDays); err != nil {
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return userID, nil
}

func (r *Repository) ClaimRedeemCode(ctx context.Context, userID, code string) (domain.RedeemCodeResponse, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.RedeemCodeResponse{}, err
	}
	defer tx.Rollback(ctx)

	normalized := strings.TrimSpace(strings.ToUpper(code))
	var redeemCodeID string
	var rewardType string
	var bonusCredits int
	var packageID string
	var packageName string
	var dailyQuota int
	var durationDays int
	err = tx.QueryRow(ctx, `
SELECT id::text, reward_type, bonus_credits, package_id, package_name, daily_quota, duration_days
FROM redeem_codes
WHERE code = $1
  AND is_active = TRUE
  AND (expires_at IS NULL OR expires_at > NOW())
  AND claimed_count < max_claims
FOR UPDATE
`, normalized).Scan(&redeemCodeID, &rewardType, &bonusCredits, &packageID, &packageName, &dailyQuota, &durationDays)
	if err != nil {
		if err == pgx.ErrNoRows {
			return domain.RedeemCodeResponse{}, fmt.Errorf("redeem code not found or inactive")
		}
		return domain.RedeemCodeResponse{}, err
	}

	var alreadyClaimed bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM redeem_code_claims
  WHERE redeem_code_id = $1::uuid
    AND user_id = $2::uuid
)
`, redeemCodeID, userID).Scan(&alreadyClaimed); err != nil {
		return domain.RedeemCodeResponse{}, err
	}
	if alreadyClaimed {
		return domain.RedeemCodeResponse{}, fmt.Errorf("redeem code already claimed")
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO redeem_code_claims (id, redeem_code_id, user_id, bonus_credits)
VALUES ($1::uuid, $2::uuid, $3::uuid, $4)
`, uuid.NewString(), redeemCodeID, userID, bonusCredits); err != nil {
		return domain.RedeemCodeResponse{}, err
	}

	if _, err := tx.Exec(ctx, `
UPDATE redeem_codes
SET claimed_count = claimed_count + 1
WHERE id = $1::uuid
`, redeemCodeID); err != nil {
		return domain.RedeemCodeResponse{}, err
	}

	balance, err := r.getUserCreditBalanceTx(ctx, tx, userID)
	if err != nil {
		return domain.RedeemCodeResponse{}, err
	}

	response := domain.RedeemCodeResponse{
		Code:          normalized,
		RewardType:    rewardType,
		BonusCredits:  0,
		CreditBalance: balance,
	}

	switch rewardType {
	case "membership":
		membership, err := r.activateMembershipTx(ctx, tx, userID, packageID, packageName, dailyQuota, durationDays)
		if err != nil {
			return domain.RedeemCodeResponse{}, err
		}
		response.ActivatedMembership = membership
		response.Message = fmt.Sprintf("%s 会员已激活", packageName)
	default:
		if err := tx.QueryRow(ctx, `
UPDATE users
SET credit_balance = credit_balance + $2
WHERE id = $1::uuid
RETURNING credit_balance
`, userID, bonusCredits).Scan(&balance); err != nil {
			return domain.RedeemCodeResponse{}, err
		}
		response.BonusCredits = bonusCredits
		response.CreditBalance = balance
		response.Message = fmt.Sprintf("获得 %d 免费 credits", bonusCredits)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.RedeemCodeResponse{}, err
	}

	return response, nil
}

func (r *Repository) ListRechargeOrders(ctx context.Context, userID string) ([]domain.RechargeOrderResponse, error) {
	rows, err := r.pool.Query(ctx, `
SELECT id::text, status, package_id, payment_method, amount_cny, daily_quota, duration_days
FROM recharge_orders
WHERE user_id = $1::uuid
ORDER BY created_at DESC
LIMIT 12
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.RechargeOrderResponse
	for rows.Next() {
		var item domain.RechargeOrderResponse
		if err := rows.Scan(&item.OrderID, &item.Status, &item.PackageID, &item.PaymentMethod, &item.AmountCNY, &item.DailyQuota, &item.DurationDays); err != nil {
			return nil, err
		}
		item.MockPayReady = item.PaymentMethod == "alipay" && item.Status == "pending"
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) ListUsageRecords(ctx context.Context, userID string) ([]domain.UsageRecord, error) {
	rows, err := r.pool.Query(ctx, `
SELECT id::text, provider, symbol, cost_credits, membership_quota_used, bonus_credits_used, created_at::text
FROM usage_records
WHERE user_id = $1::uuid
ORDER BY created_at DESC
LIMIT 20
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.UsageRecord
	for rows.Next() {
		var item domain.UsageRecord
		if err := rows.Scan(&item.ID, &item.Provider, &item.Symbol, &item.CostCredits, &item.QuotaUsed, &item.BonusUsed, &item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) ConsumeCredits(ctx context.Context, userID, provider, symbol string, costCredits int) (int, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
UPDATE users
SET credit_balance = credit_balance - $2
WHERE id = $1::uuid
  AND credit_balance >= $2
`, userID, costCredits)
	if err != nil {
		return 0, err
	}
	if tag.RowsAffected() == 0 {
		return 0, fmt.Errorf("insufficient credits")
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO usage_records (id, user_id, provider, symbol, cost_credits, membership_quota_used, bonus_credits_used)
VALUES ($1::uuid, $2::uuid, $3, $4, $5, 0, $5)
`, uuid.NewString(), userID, provider, symbol, costCredits); err != nil {
		return 0, err
	}

	var balance int
	if err := tx.QueryRow(ctx, `
SELECT credit_balance
FROM users
WHERE id = $1::uuid
`, userID).Scan(&balance); err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return balance, nil
}

func (r *Repository) ConsumeAIAllowance(ctx context.Context, userID, provider, symbol string, costCredits int) (domain.AIAllowanceStatus, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.AIAllowanceStatus{}, err
	}
	defer tx.Rollback(ctx)

	var balance int
	if err := tx.QueryRow(ctx, `
SELECT credit_balance
FROM users
WHERE id = $1::uuid
FOR UPDATE
`, userID).Scan(&balance); err != nil {
		return domain.AIAllowanceStatus{}, err
	}

	var membershipID string
	var membership domain.MembershipStatus
	membershipErr := tx.QueryRow(ctx, `
SELECT id::text, package_id, package_name, status, daily_quota, duration_days, starts_at::text, ends_at::text
FROM user_memberships
WHERE user_id = $1::uuid
  AND status = 'active'
  AND starts_at <= NOW()
  AND ends_at > NOW()
ORDER BY ends_at DESC, created_at DESC
LIMIT 1
FOR UPDATE
`, userID).Scan(
		&membershipID,
		&membership.PackageID,
		&membership.PackageName,
		&membership.Status,
		&membership.DailyQuota,
		&membership.DurationDays,
		&membership.StartsAt,
		&membership.EndsAt,
	)

	var quotaUsedToday int
	if membershipErr == nil {
		if err := tx.QueryRow(ctx, `
SELECT COALESCE(SUM(membership_quota_used), 0)
FROM usage_records
WHERE user_id = $1::uuid
  AND created_at::date = CURRENT_DATE
`, userID).Scan(&quotaUsedToday); err != nil {
			return domain.AIAllowanceStatus{}, err
		}
	} else if membershipErr != pgx.ErrNoRows {
		return domain.AIAllowanceStatus{}, membershipErr
	}

	quotaRemaining := 0
	var currentMembership *domain.MembershipStatus
	if membershipErr == nil {
		quotaRemaining = membership.DailyQuota - quotaUsedToday
		if quotaRemaining < 0 {
			quotaRemaining = 0
		}
		currentMembership = &membership
	}

	quotaUsed := costCredits
	if quotaUsed > quotaRemaining {
		quotaUsed = quotaRemaining
	}
	bonusUsed := costCredits - quotaUsed
	if bonusUsed > balance {
		return domain.AIAllowanceStatus{}, fmt.Errorf("insufficient credits")
	}

	newBalance := balance
	if bonusUsed > 0 {
		if err := tx.QueryRow(ctx, `
UPDATE users
SET credit_balance = credit_balance - $2
WHERE id = $1::uuid
RETURNING credit_balance
`, userID, bonusUsed).Scan(&newBalance); err != nil {
			return domain.AIAllowanceStatus{}, err
		}
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO usage_records (id, user_id, provider, symbol, cost_credits, membership_quota_used, bonus_credits_used)
VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7)
`, uuid.NewString(), userID, provider, symbol, costCredits, quotaUsed, bonusUsed); err != nil {
		return domain.AIAllowanceStatus{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.AIAllowanceStatus{}, err
	}

	todayQuota := domain.DailyQuotaStatus{
		Date:      time.Now().Format("2006-01-02"),
		Total:     0,
		Used:      0,
		Remaining: 0,
	}
	if currentMembership != nil {
		todayQuota.Total = currentMembership.DailyQuota
		todayQuota.Used = quotaUsedToday + quotaUsed
		todayQuota.Remaining = currentMembership.DailyQuota - todayQuota.Used
		if todayQuota.Remaining < 0 {
			todayQuota.Remaining = 0
		}
	}

	return domain.AIAllowanceStatus{
		CreditBalance:      newBalance,
		CurrentMembership:  currentMembership,
		TodayQuota:         todayQuota,
		AvailableToConsume: newBalance + todayQuota.Remaining,
	}, nil
}
