# Architecture Notes

## Current Runtime Shape

- `frontend`: React + Vite single-page app
- `api-go`: auth, symbols, OHLC, proxy to AI service
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
- `rate:ai:user:{userId}`
  - owner: `ai-go`
  - ttl: 1 minute
- `rate:ai:ip:{ip}`
  - owner: `ai-go`
  - ttl: 1 minute
- `auth:token:blacklist:{jti}`
  - owner: `api-go`
  - ttl: matches JWT lifetime

## AI Usage Policy

AI usage is unlimited. Users provide their own AI keys or use server-configured providers.
Rate limiting is applied per-user and per-IP to prevent abuse.

## K8s Direction

For Kubernetes, prefer:

- managed PostgreSQL
- managed Redis
- app deployments only inside the cluster

The included manifests are a base shape, not a production-complete cluster package.
