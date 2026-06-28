# Market Data

KoalaTrade treats market data as a server-owned capability. The browser must never call Finnhub, CoinGecko, Polymarket, or similar providers directly.

## Current Providers

`MARKET_DATA_PROVIDER=mock` is the default provider. It returns deterministic BTC, SPY, GLD, and event-market fixtures through:

- `GET /api/markets`
- `GET /api/quotes?ids=crypto:btc,etf:spy`

`MARKET_DATA_PROVIDER=coingecko` enables a server-side CoinGecko overlay for BTC while keeping non-crypto assets on the mock provider. If CoinGecko fails or rate-limits, BTC falls back to mock data.

CoinGecko configuration:

```bash
MARKET_DATA_PROVIDER=coingecko
MARKET_DATA_CACHE_SECONDS=60
MARKET_DATA_HTTP_TIMEOUT_SECONDS=5
COINGECKO_API_KEY=
```

`COINGECKO_API_KEY` is optional for the Demo API. When present, the backend sends it as `x-cg-demo-api-key`.

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
- CoinGecko: crypto spot prices. BTC is implemented.
- Polymarket CLOB: read-only event markets and prices.

The next provider implementation should use the existing `marketdata.Provider` interface without changing the frontend API contract.

## Live Smoke

Run the backend with CoinGecko enabled:

```bash
MARKET_DATA_PROVIDER=coingecko go run ./cmd/server
```

Then request:

```bash
curl 'http://127.0.0.1:8080/api/quotes?ids=crypto:btc'
```

Expected result: `source` is `coingecko` when the live request succeeds, otherwise `mock` via fallback.
