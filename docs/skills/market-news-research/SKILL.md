---
name: market-news-research
description: Use when the task requires symbol-specific news or policy context for a selected time range. This skill defines how agents in this repo fetch, filter, and ground market news before passing it into AI analysis.
---

# Market News Research

Use this skill when the product needs news context for:

- a selected symbol
- a selected date range
- support or resistance reasoning
- risk explanation
- policy or industry catalyst review

Do not use this skill for pure chart analysis.

## Required Behavior

- treat fetched news as evidence, not as the final answer
- keep source facts separate from model inference
- prefer concise structured extraction over article-style summary
- reject unsupported claims

## Input Contract

Expected inputs:

- `symbol`
- `start_date`
- `end_date`
- optional `market`
- optional `question`

## Research Workflow

1. Resolve the research target from chart context:
   - symbol
   - date range
   - whether the user is asking about catalyst, risk, or direction
2. Fetch candidate news through controlled backend functions or adapters.
3. Normalize each item into:
   - `title`
   - `published_at`
   - `source`
   - `url`
   - `summary`
   - `relevance_reason`
4. Drop duplicates and low-signal items.
5. Return structured evidence, not narrative prose.
6. Pass only the reduced evidence set into the model prompt.

## Output Shape

Preferred evidence payload:

```json
{
  "symbol": "000001",
  "window": {
    "start_date": "2026-03-01",
    "end_date": "2026-03-22"
  },
  "items": [
    {
      "title": "string",
      "published_at": "ISO-8601",
      "source": "string",
      "url": "string",
      "summary": "string",
      "relevance_reason": "string"
    }
  ]
}
```

## Hard Rules

- never fabricate a headline
- never fabricate a source URL
- never claim recency without fetched timestamps
- never mix chart inference into fetched evidence fields
- if no grounded news exists, return an empty list and say so explicitly

## Provider Independence

This skill must work the same way whether the final model is:

- OpenAI
- DeepSeek
- Gemini

The provider may generate the explanation, but it must consume the same structured evidence.
