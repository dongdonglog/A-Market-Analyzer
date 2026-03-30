package cache

import (
	"context"
	"encoding/json"
	"time"

	"market-project/backend/internal/config"

	redis "github.com/redis/go-redis/v9"
)

type Client struct {
	rdb *redis.Client
}

func New(cfg config.Config) *Client {
	if !cfg.RedisEnabled || cfg.RedisAddr == "" {
		return &Client{}
	}

	return NewFromOptions(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
}

func NewFromOptions(addr, password string, db int) *Client {
	return &Client{
		rdb: redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password,
			DB:       db,
		}),
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.rdb != nil
}

func (c *Client) Ping(ctx context.Context) error {
	if !c.Enabled() {
		return nil
	}
	return c.rdb.Ping(ctx).Err()
}

func (c *Client) GetJSON(ctx context.Context, key string, dest any) (bool, error) {
	if !c.Enabled() {
		return false, nil
	}

	value, err := c.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal([]byte(value), dest); err != nil {
		return false, err
	}

	return true, nil
}

func (c *Client) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	if !c.Enabled() {
		return nil
	}

	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return c.rdb.Set(ctx, key, payload, ttl).Err()
}

func (c *Client) IncrementWindow(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	if !c.Enabled() {
		return 0, nil
	}

	pipe := c.rdb.TxPipeline()
	count := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}

	return count.Val(), nil
}

func (c *Client) Delete(ctx context.Context, keys ...string) error {
	if !c.Enabled() || len(keys) == 0 {
		return nil
	}

	return c.rdb.Del(ctx, keys...).Err()
}

func (c *Client) SetString(ctx context.Context, key, value string, ttl time.Duration) error {
	if !c.Enabled() {
		return nil
	}

	return c.rdb.Set(ctx, key, value, ttl).Err()
}

func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	if !c.Enabled() {
		return false, nil
	}

	count, err := c.rdb.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
