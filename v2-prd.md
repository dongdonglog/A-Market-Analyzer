# V2 PRD: Eastmoney AI Market Copilot

## 1. Product Summary

V2 is a market analysis product built around Eastmoney data, chart reading, AI explanation, and short-term outlook generation.

The first shippable version is not a general-purpose quant platform. It is a focused MVP:

- fetch market data from Eastmoney
- verify the pipeline with 2 to 3 target symbols
- render chart data in a clean web UI
- let users ask AI about a symbol or a selected range
- return both explanation and forward-looking judgment
- require login before using the product

Core promise:

`Open a stock, inspect the move, and get AI explanation plus next-step judgment.`

## 2. Problem

The project needs a tighter scope so it can ship in one night.

Previous planning had three major problems:

- it depended on CSV import instead of the actual target data source
- it avoided prediction and auth even though they are part of the desired product
- it did not commit to a final stack for fast execution

This version fixes that by choosing one concrete data source, one concrete stack, and one concrete MVP loop.

## 3. Goals

Goals for the overnight MVP:

- fetch daily OHLC market data from Eastmoney
- verify the end-to-end path with 2 to 3 symbols
- display a symbol chart in the frontend
- allow users to select a date range
- allow users to ask AI questions about the symbol or selected range
- return AI output that includes explanation and short-term outlook
- require auth before entering the main product
- use a clean Go backend and React frontend with clear boundaries

## 4. Non-Goals

Explicitly out of scope for this overnight MVP:

- news crawling and news-based reasoning
- real-time streaming quotes
- high-frequency or intraday trading workflows
- broker integration
- complex multi-role permission systems
- portfolio management
- advanced backtesting engine
- self-trained quantitative prediction models

## 5. Target Users

- individual traders
- internal market researchers
- users who want a fast AI-assisted read on a few tracked symbols

## 6. Core User Flows

### Flow A: Login

1. User opens the app
2. User signs in
3. Frontend stores auth state
4. User enters the main workspace

### Flow B: View Symbol

1. User opens the symbol list
2. User selects one tracked symbol
3. Backend returns Eastmoney-backed historical OHLC data
4. Frontend renders the candlestick chart

### Flow C: Ask AI About A Symbol

1. User opens a symbol
2. User asks a question such as “What matters most here?”
3. Backend summarizes recent OHLC structure and indicators
4. Backend calls the configured OpenAI-compatible API
5. Frontend displays explanation and outlook

### Flow D: Ask AI About A Selected Range

1. User selects a date range on the chart
2. User asks “What happened here and what should I watch next?”
3. Backend computes range statistics and context
4. AI returns observations, risks, and next-step judgment

## 7. Functional Requirements

### Frontend

- login page
- app layout based on React and Ant Design
- tracked symbol list
- candlestick chart
- visible date range selection
- AI chat panel with multi-turn history
- loading, empty, and error states

### Backend

- auth endpoints
- Eastmoney market data fetch and normalization
- tracked symbol list endpoint
- OHLC query endpoint
- AI query endpoint
- health endpoint

### AI

- OpenAI-compatible config only
- system prompt specialized for chart analysis and outlook generation
- structured output with explanation, bullish/bearish bias, key points, and risk points

## 8. Data Requirements

For the first version, normalized market rows must contain:

- `symbol`
- `market`
- `date`
- `open`
- `high`
- `low`
- `close`
- `volume`

Optional derived fields may include:

- `amount`
- `amplitude`
- `change_rate`
- `turnover_rate`

## 9. Prediction Scope

“预判” in this MVP means lightweight AI judgment, not a full predictive model.

The output should include:

- what the recent structure suggests
- the current directional bias: `bullish`, `bearish`, or `neutral`
- the key confirmation signals
- the invalidation or risk signals
- a short next-step watchlist

## 10. Auth Scope

Auth in this MVP should stay simple:

- email and password login
- JWT or equivalent token-based session
- one basic user role is enough for V1

Do not over-design a full permission matrix.

## 11. Success Metrics

- a new developer can run backend and frontend locally in one setup session
- the app can fetch and display 2 to 3 Eastmoney-backed symbols successfully
- login works for the main protected route
- the first AI response returns successfully after setup
- the app can answer both symbol-level and range-level questions

## 12. Release Criteria For The Overnight MVP

- Go backend starts successfully
- PostgreSQL schema is initialized
- React + Ant Design frontend starts successfully
- auth works for the protected app shell
- Eastmoney fetch works for 2 to 3 target symbols
- chart loads symbol OHLC data
- AI panel returns explanation plus outlook
- README documents setup and run flow clearly
