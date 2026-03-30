# V2 Overnight Build Plan

## Objective

Ship a working overnight MVP with:

- `apps/frontend`: React + Ant Design web app
- `apps/backend`: Go HTTP API
- PostgreSQL for storage
- Eastmoney as the market data source
- auth, chart analysis, and AI outlook

The system only needs to support 2 to 3 tracked symbols for initial verification.

## Principles

- build the shortest end-to-end working path first
- use one real data source: Eastmoney
- avoid feature creep
- prefer direct implementation over generic abstractions
- keep auth and prediction simple enough to ship tonight

## Architecture

### Frontend

- React + TypeScript + Vite
- Ant Design for layout and components
- pages:
  - login
  - dashboard
- dashboard areas:
  - tracked symbol list
  - main chart
  - AI chat and outlook panel

### Backend

- Go HTTP API
- modules:
  - auth
  - config
  - db
  - eastmoney
  - symbols
  - ohlc
  - ai
  - health

### Database

- PostgreSQL
- store users, tracked symbols, OHLC history, AI sessions, AI messages

## Proposed Directory Layout

```text
apps/
  frontend/
  backend/
docs/
  v2-prd.md
  v2-plan.md
  api.md
```

## MVP Workstreams

### 1. Foundation

- create frontend and backend skeletons
- add env loading
- add health endpoint
- add PostgreSQL connection

### 2. Auth

- define user table
- implement login API
- add password hashing
- add JWT middleware
- protect main app routes

### 3. Eastmoney Data Pipeline

- define the Eastmoney fetch client
- normalize response into internal OHLC shape
- support 2 to 3 target symbols
- store historical rows in PostgreSQL

### 4. Read APIs

- `GET /health`
- `POST /auth/login`
- `GET /symbols`
- `GET /symbols/:symbol/ohlc`
- `POST /copilot/query`

### 5. AI Query API

- accept symbol, optional date range, question, and chat history
- compute recent statistics and range statistics
- build chart-analysis prompt
- return explanation plus outlook

### 6. Frontend

- login page
- protected dashboard shell
- symbol list
- chart area
- range selection state
- AI chat and structured result display

### 7. Polish

- loading states
- empty states
- error states
- README
- local smoke test

## Data Model

### `users`

- `id`
- `email`
- `password_hash`
- `created_at`

### `symbols`

- `id`
- `symbol`
- `name`
- `market`
- `source`
- `created_at`

### `ohlc_daily`

- `id`
- `symbol`
- `market`
- `date`
- `open`
- `high`
- `low`
- `close`
- `volume`
- `amount`
- `change_rate`
- `created_at`

### `ai_sessions`

- `id`
- `user_id`
- `symbol`
- `start_date`
- `end_date`
- `created_at`

### `ai_messages`

- `id`
- `session_id`
- `role`
- `content`
- `created_at`

## API Draft

### `GET /health`

Returns service status.

### `POST /auth/login`

Request body:

```json
{
  "email": "demo@example.com",
  "password": "string"
}
```

Response body:

```json
{
  "token": "string",
  "user": {
    "id": "string",
    "email": "demo@example.com"
  }
}
```

### `GET /symbols`

Returns tracked symbols.

### `GET /symbols/{symbol}/ohlc`

Returns OHLC rows for one symbol.

### `POST /copilot/query`

Request body:

```json
{
  "symbol": "600519",
  "start_date": "2026-03-01",
  "end_date": "2026-03-18",
  "question": "What happened here and what should I watch next?",
  "history": [
    {"role": "user", "content": "Give me the big picture"},
    {"role": "assistant", "content": "The range shows..."}
  ]
}
```

Response body:

```json
{
  "answer": "string",
  "bias": "bullish",
  "key_points": ["string"],
  "risk_points": ["string"],
  "watch_items": ["string"]
}
```

## Milestones

### Milestone 1: Skeleton

- frontend and backend apps created
- PostgreSQL connection works
- health endpoint works

### Milestone 2: Auth + Data

- login works
- Eastmoney fetch works for 2 to 3 symbols
- OHLC data is stored and queryable

### Milestone 3: UI + AI

- chart renders symbol data
- range selection works
- AI endpoint returns explanation plus outlook

### Milestone 4: Demo Ready

- protected dashboard is usable
- loading and error states exist
- README supports local setup

## Execution Order

1. scaffold Go backend
2. scaffold React + Ant Design frontend
3. wire PostgreSQL
4. implement auth
5. implement Eastmoney fetch and persistence
6. expose symbols and OHLC APIs
7. render symbol list and chart
8. add range selection
9. implement AI query API
10. connect AI panel
11. finish README and smoke test

## Risks

- Eastmoney response shape may need field mapping adjustments
- auth can bloat if not kept minimal
- prediction can bloat if treated like quantitative modeling
- PostgreSQL setup can slow execution if environment is not ready

## Guardrails

- no news system in this overnight build
- no real-time market stream
- no broker integration
- no complex role and permission matrix
- no advanced prediction engine beyond AI outlook

## First Night Deliverables

- backend skeleton
- frontend skeleton
- PostgreSQL schema
- login flow
- Eastmoney-backed symbol data for 2 to 3 symbols
- one symbol visible on chart
- one AI answer with outlook returned successfully
- setup and run documentation
