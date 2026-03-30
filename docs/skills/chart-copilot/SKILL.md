---
name: chart-copilot
description: Use when the task is chart-grounded market explanation, support or resistance analysis, risk review, or short-term outlook for a selected symbol and range. This skill defines how repo agents combine OHLC indicators and grounded research.
---

# Chart Copilot

Use this skill for the main AI panel in this repo.

## Goal

Produce a grounded answer that combines:

- selected chart range
- OHLC structure
- MA, RSI, MACD, KDJ, BOLL
- optional grounded news evidence from `market-news-research`

## Input Contract

Expected inputs:

- `symbol`
- `start_date`
- `end_date`
- `question`
- `ohlc_rows`
- `indicator_snapshot`
- optional `news_evidence`

## Reasoning Order

1. Read the chart structure first:
   - trend
   - momentum
   - volatility
   - recent highs and lows
2. Derive key levels:
   - support
   - pressure
   - risk
3. If grounded news exists, explain whether it confirms, weakens, or conflicts with the chart.
4. Produce a concise answer for traders.

## Output Shape

Preferred response:

```json
{
  "answer": "string",
  "bias": "bullish",
  "key_points": ["string"],
  "risk_points": ["string"],
  "watch_items": ["string"],
  "levels": {
    "support": {
      "value": 10.80,
      "reason": "string"
    },
    "pressure": {
      "value": 11.20,
      "reason": "string"
    },
    "risk": {
      "value": 10.55,
      "reason": "string"
    }
  },
  "news_context": {
    "used": true,
    "count": 3
  }
}
```

## Hard Rules

- no fabricated price levels
- no fabricated indicators
- no fabricated news linkage
- if news is unavailable, say the answer is chart-only
- keep chart facts and model judgment distinct

## Repo Policy

This skill is provider-agnostic.

Do not write prompts that depend on:

- a single vendor SDK
- a single model family
- free-form browsing inside the model
