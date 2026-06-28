# Market Data

KoalaTrade treats market data as a server-owned capability. The browser must never call Finnhub, CoinGecko, Polymarket, or similar providers directly.

## Current Provider

`MARKET_DATA_PROVIDER=mock` is the only active provider. It returns deterministic BTC, SPY, GLD, and event-market fixtures through:

- `GET /api/markets`
- `GET /api/quotes?ids=crypto:btc,etf:spy`

The frontend consumes `/api/markets` and falls back to local fixture data if the backend is unavailable.

## Provider Rules

- Keep API keys on the server only.
- Do not add third-party browser SDKs.
- Cache provider responses server-side before they reach clients.
- Normalize all providers into the same `assetId`, `symbol`, `kind`, `priceCents`, and `changeBps` shape.
- Do not send user identifiers, portfolio contents, watchlists, or trade history to market-data providers.
- Prefer batched fetches and provider-specific rate-limit guards.

## Planned Provider Split

- Finnhub: stocks, ETFs, and commodities.
- CoinGecko: crypto spot prices.
- Polymarket CLOB: read-only event markets and prices.

The next implementation step should add one provider behind the existing `marketdata.Provider` interface without changing the frontend API contract.
