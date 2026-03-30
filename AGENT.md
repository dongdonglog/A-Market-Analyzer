# AGENT.md

## Mission

Ship an overnight MVP of Eastmoney AI Market Copilot.

The product is a protected web app that fetches market data from Eastmoney, shows chart data for a small tracked symbol set, and lets users ask AI for explanation plus short-term outlook.

## Source Of Truth

- read `v2-prd.md` first
- read `v2-plan.md` second
- if documents conflict, prefer:
  1. `v2-prd.md` for scope
  2. `v2-plan.md` for execution order
  3. this file for engineering defaults

## Product Invariants

Unless the user changes the goal again, these are fixed:

- data source is Eastmoney, not CSV import
- initial validation only needs 2 to 3 symbols
- frontend uses React and Ant Design
- backend uses Go
- database uses PostgreSQL
- auth is required
- market data access is required
- AI must provide explanation and forward-looking judgment
- no real-time stream in this build

## Repo Intent

- this repo is effectively greenfield
- do not preserve old assumptions from previous planning
- optimize for a working demo by tonight
- prefer a thin vertical slice over broad infrastructure

## Default Technical Choices

Use these defaults unless the codebase later creates a stronger convention:

- frontend: `React + TypeScript + Vite`
- UI library: `Ant Design`
- frontend routing: `react-router`
- frontend data fetching: `@tanstack/react-query`
- charting: prefer `lightweight-charts`; only add another chart library if it solves a concrete blocker
- backend: `Go`
- HTTP router: `gin` or `fiber`; prefer the simpler option already established in the repo if one appears
- config: env-based config loading
- database: `PostgreSQL`
- SQL access: use a practical Go Postgres stack with migrations
- auth: email/password login with JWT
- AI access: OpenAI-compatible HTTP integration

Choose fast, debuggable, common tools. Do not over-engineer.

## Target Layout

```text
apps/
  frontend/
  backend/
docs/
  v2-prd.md
  v2-plan.md
  api.md
```

Add more directories only when they clearly reduce friction.

## Required Backend Capabilities

Backend must provide:

- `GET /health`
- `POST /auth/login`
- `GET /symbols`
- `GET /symbols/:symbol/ohlc`
- `POST /copilot/query`

Backend modules should stay explicit:

- `auth`
- `config`
- `db`
- `eastmoney`
- `symbols`
- `ohlc`
- `ai`
- `health`

Do not mix Eastmoney fetch logic, AI prompt building, and HTTP handlers in one large file.

## Required Frontend Capabilities

Frontend must provide:

- login page
- protected app shell
- tracked symbol list
- main chart area
- visible range selection state
- AI chat and outlook panel
- loading states
- empty states
- backend error states

## Prediction Rule

“预判” for this repo means lightweight AI outlook, not a quantitative forecasting engine.

Expected output shape should trend toward:

```json
{
  "answer": "string",
  "bias": "bullish",
  "key_points": ["string"],
  "risk_points": ["string"],
  "watch_items": ["string"]
}
```

If a richer model is later needed, the user must ask for it explicitly.

## Agent Rule

This repo should support multiple model providers:

- `OpenAI`
- `DeepSeek`
- `Gemini`

Provider choice must stay behind one backend AI interface. The frontend must not depend on a single vendor SDK.

## Tool-Grounded Research Rule

When AI needs external context such as market news, policy headlines, or public company updates:

- do not let the model invent facts
- do not let the model browse arbitrary sites without a controlled tool path
- route research through backend-controlled tools or explicit fetch functions
- persist the raw fetched source metadata when practical
- make the final answer distinguish:
  - chart-derived judgment
  - news-derived context
  - model inference

In short:

- model providers can change
- research workflow should not change
- all providers must follow the same grounded tool path

## Repo Skills

Repo-local skills should live under:

```text
docs/skills/
```

These skills are not vendor-specific prompts. They are operating instructions for how the agent should work inside this repo.

Current planned repo skills:

- `market-news-research`
- `chart-copilot`

Each skill should define:

- when it should trigger
- what inputs it may trust
- what tools or backend functions it may call
- what output shape it must return
- what it must never fabricate

## News Research Rule

News support is allowed only under these constraints:

- no real-time requirement
- focus on selected symbol plus selected time range
- prefer structured outputs over long article summaries
- keep source links, titles, publish times, and extracted claims separate from AI judgment

The model should never say “news says” unless the fetched source metadata exists.

## Auth Rule

Keep auth minimal enough to ship:

- email and password
- password hash storage
- JWT-based session
- one basic user role is enough

Do not build a large RBAC system unless the user asks for it.

## Execution Order

Build in this order unless a blocker forces a change:

1. backend skeleton
2. frontend skeleton
3. PostgreSQL wiring
4. auth flow
5. Eastmoney fetch and persistence
6. symbols and OHLC APIs
7. chart rendering
8. range selection
9. AI query endpoint
10. AI panel integration
11. README and smoke test

Each step should leave the repo more runnable than before.

## Engineering Rules

- make direct code changes instead of only proposing them
- keep modules small and explicit
- add only the minimum abstraction needed for the current step
- prefer practical local validation after each meaningful slice
- update docs when setup, APIs, or architecture changes
- call out scope creep immediately

## Definition Of Done

A task is not done until:

- code is implemented
- validation exists at the input boundary
- the changed flow is tested locally when practical
- obvious loading and error behavior is handled
- docs are updated if the setup or contract changed

## Skill Usage

- use `openai-docs` when current OpenAI API behavior or model guidance matters
- use `playwright` and `screenshot` for frontend verification when the UI is running
- use `security-best-practices` when reviewing auth, token handling, or secret usage
- use `doc` when generating or restructuring repo documentation
