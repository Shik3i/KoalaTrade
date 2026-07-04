# Architecture

KoalaTrade is a local-first paper-trading app. The browser owns the portfolio;
the server owns all third-party API traffic and optional sync.

```
Browser (Svelte SPA)
├── IndexedDB: portfolio, transactions, preferences, device id
├── localStorage: admin token
└── fetch /api/* (same-origin)
        │
        ▼
Go API (chi) ── SQLite (WAL)
├── marketdata  → mock | CoinGecko | Finnhub   (quote cache + staggered poller)
├── esports     → lolesports schedule/teams + Polymarket odds (TTL cache, results persisted)
├── storage     → portfolios, sync snapshots, team_mappings, users, app_meta (KV)
└── auth        → PBKDF2 hashing + HMAC bearer tokens (admin)
        │
        ▼
External APIs: esports-api.lolesports.com · gamma-api.polymarket.com · finnhub.io · coingecko
```

## Principles

- **Local-first**: trading works with no account; the portfolio lives in IndexedDB. Sync is opt-in and device-scoped.
- **Server owns external traffic**: clients never call market/odds providers directly — keeps API keys private and centralizes caching/rate-limiting.
- **Privacy-first**: no CDN, fonts, analytics, or third-party client SDKs.

## Backend layout (`backend/internal/`)

| Package | Responsibility |
|---|---|
| `config` | Environment configuration |
| `server` | chi router, handlers, middleware, pollers, admin |
| `marketdata` | Provider chain (Finnhub → Yahoo → CoinGecko → registry catalogue), keyless by default, quote cache |
| `esports` | Schedule, Polymarket odds, slug mapping, results, status |
| `storage` | SQLite access (portfolios, snapshots, mappings, users, meta) |
| `auth` | Password hashing + signed tokens |

## Frontend layout (`frontend/src/`)

- `App.svelte` — shell, views (Trade/Portfolio/Markets/eSports/Profile/Admin), state.
- `lib/components/` — `AreaChart`, `EsportsView`, `ProfileView`, `AdminView`, `Toasts`.
- `lib/` — `api.ts` (typed client), `portfolio.ts` (trade engine + analytics), `portfolio-db.ts` (IndexedDB), `preferences.ts`, `toast.ts`.

## Rate limiting

The market-data poller refreshes assets in **staggered batches** so that, over
`MARKET_DATA_REFRESH_WINDOW_SECONDS`, every asset is refreshed exactly once —
spreading provider calls evenly to respect free-tier per-minute limits. Polymarket
has no rate limit, so odds are refreshed on demand right before a bet.
