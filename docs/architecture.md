# Architecture Notes

## Current Runtime Shape

- `frontend`: React + Vite single-page app
- `api-go`: auth, symbols, billing, token revocation, proxy to AI service
- `ai-go`: AI providers, copilot query flow, session storage, rate limiting
- `postgres`: shared primary data store
- `redis`: cache, short-lived session reads, token blacklist, rate limiting

## Redis Key Policy

- `cache:symbols:list`
  - owner: `api-go`
  - ttl: 5 minutes
  - invalidation: symbol refresh, symbol delete
- `cache:ohlc:{symbol}:{start}:{end}`
  - owner: `api-go`
  - ttl: 2 minutes
  - invalidation: passive ttl only for now
- `cache:ai:providers`
  - owner: `ai-go`
  - ttl: 5 minutes
- `cache:copilot:sessions:{userId}:{symbol}`
  - owner: `ai-go`
  - ttl: 1 minute
  - invalidation: new copilot message, favorite toggle
- `cache:copilot:messages:{userId}:{sessionId}`
  - owner: `ai-go`
  - ttl: 1 minute
  - invalidation: new copilot message, favorite toggle
- `cache:billing:user:{userId}`
  - owner: `api-go`
  - ttl: 1 minute
  - invalidation: recharge order create, copilot usage
- `rate:ai:user:{userId}`
  - owner: `ai-go`
  - ttl: 1 minute
- `rate:ai:ip:{ip}`
  - owner: `ai-go`
  - ttl: 1 minute
- `auth:token:blacklist:{jti}`
  - owner: `api-go`
  - ttl: matches JWT lifetime

## Membership Direction

Current implementation:

- three fixed plans: `starter`, `active`, `pro`
- each plan grants a 30-day membership window
- each membership has a durable daily quota stored in PostgreSQL
- AI usage consumes membership daily quota first, then falls back to free `credits`
- redeem codes only add free `credits`
- current local demo flow activates the membership immediately after order creation

Still worth adding later:

1. real payment callback to switch order state before activation
2. admin-side redeem code generation and disable flow
3. package config moved from code to database or config service
4. daily quota reset/aggregation metrics exposed to monitoring

## Payment Direction

Current state:

- `api-go` owns recharge order creation and payment status transitions
- Alipay is wired as a mock QR-code flow first
- order creation only inserts a `pending` order
- membership activation happens only when the payment callback path marks the order as `paid`
- a mock callback endpoint exists so frontend and local testing can exercise the full flow

Next production step:

1. replace mock Alipay provider with real precreate API
2. verify callback signature
3. persist provider trade number and callback payload
4. add refund and close-order handling

## K8s Direction

For Kubernetes, prefer:

- managed PostgreSQL
- managed Redis
- app deployments only inside the cluster

The included manifests are a base shape, not a production-complete cluster package.
