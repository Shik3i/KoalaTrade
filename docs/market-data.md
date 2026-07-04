# Market Data

KoalaTrade treats market data as a server-owned capability. The browser must never call Finnhub, CoinGecko, Polymarket, or similar providers directly.

## Architecture

Market data is split into two concerns:

1. **Catalogue** — the `RegistryProvider` (`internal/marketdata/registry.go`) is the single source of truth for every tradable asset: its `assetId`, `symbol`, `name`, `kind`, and intended `source`. It carries no live prices. Adding a stock/ETF/commodity/crypto means editing this one list.
2. **Live prices** — the provider chain enriches quotes on demand. `Finnhub` (stocks, ETFs, commodities) wraps `CoinGecko` (crypto) wraps the registry. Each layer serves the asset kinds it knows and delegates the rest downstream; the registry is the final fallback.

The providers are **always wired up** (there is no provider-selection switch). Whether a layer produces live data depends only on connectivity and API keys:

- CoinGecko needs no key, so the 8 crypto assets get live prices out of the box.
- Finnhub requires `FINNHUB_API_KEY`. Without it, the 121 equities/ETFs/commodities have no price yet (they display `—` until a key is configured).

Read endpoints (`GET /api/markets`, `GET /api/markets/{id}/history`, `GET /api/quotes`) **never** trigger a provider fetch — they read the catalogue plus persisted quotes from SQLite. Live prices are refreshed exclusively by the background poller.

## Staggered polling

The poller refreshes one asset per tick in a round-robin loop, spread evenly across `MARKET_DATA_REFRESH_WINDOW_SECONDS` (default 900s):

```
interval = max(3s, RefreshWindow / assetCount)
```

At one request per interval this stays well under both the Finnhub (60/min) and CoinGecko demo (30/min) free-tier limits, regardless of catalogue size. Adding assets lengthens the full-cycle time but never raises the request rate.

Each refreshed quote is written to `asset_quotes` (latest) and to `asset_history` in four rounded tiers (5M/1H/6H/1D) used to serve chart ranges.

## Configuration

```bash
MARKET_DATA_CACHE_SECONDS=60
MARKET_DATA_REFRESH_WINDOW_SECONDS=900
MARKET_DATA_HTTP_TIMEOUT_SECONDS=5
COINGECKO_API_KEY=
FINNHUB_API_KEY=
```

`COINGECKO_API_KEY` is optional for the Demo API. When present, the backend sends it as `x-cg-demo-api-key`. `FINNHUB_API_KEY` is required for Finnhub live quotes and is sent as the server-side `token` query parameter.

## Provider Rules

- Keep API keys on the server only.
- Do not add third-party browser SDKs.
- Read endpoints serve from the DB; only the poller talks to providers.
- Normalize all providers into the same `assetId`, `symbol`, `kind`, `priceCents`, and `changeBps` shape.
- Do not send user identifiers, portfolio contents, watchlists, or trade history to market-data providers.

## Live Smoke

Run the backend with a Finnhub key so equities get live prices:

```bash
FINNHUB_API_KEY=... go run ./cmd/server
```

Then request:

```bash
curl 'http://127.0.0.1:8080/api/quotes?ids=crypto:btc,etf:spy,commodity:gld'
```

Expected result once the poller has cycled: `source` is `coingecko` for BTC and `finnhub` for SPY/GLD.
