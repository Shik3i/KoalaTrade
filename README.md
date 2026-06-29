# KoalaTrade

Privacy-first paper trading for event markets, stocks, ETFs, crypto, and gold. KoalaTrade is a no-real-money trading playground: users start with virtual cash, build a portfolio, and can later opt in to sync and leaderboards.

The repository is currently in MVP stage: backend, frontend, Docker, CI, local portfolio state, simulated trades, background-polled server-side market data, optional CoinGecko/Finnhub overlays, and opt-in device-scoped portfolio sync are in place.

## Tech Stack

| Component | Choice |
|---|---|
| Frontend | Svelte 5 + Vite + TypeScript SPA/PWA |
| Styling | Local CSS variables and system fonts, no CDN assets |
| Icons | Local bundled `@lucide/svelte` package plus local SVG app icon |
| Backend | Go 1.26 + Chi Router |
| Database | SQLite with pure-Go driver, WAL enabled |
| Client storage | IndexedDB local portfolio and transaction state |
| Auth | No account required for MVP; future sessions should use secure HTTP-only cookies |
| Market data | Mock provider by default, optional CoinGecko BTC and Finnhub SPY/GLD overlays |
| Hosting | Hetzner VPS + Caddy + Docker/Compose planned |

## Current Foundation

- Go API server with `/healthz`, `/api/config`, `/api/markets`, and `/api/quotes`
- SQLite initialization with WAL, foreign keys, and busy timeout
- SQLite schema prepared for optional portfolio sync and leaderboard snapshots
- Svelte dashboard shell with local-first/privacy status
- IndexedDB local portfolio state with reset support
- Simulated buy/sell flow against local cash and positions
- Opt-in portfolio sync using a local IndexedDB device id
- Server-owned market data service with cached quote endpoint and background poller
- Mock market provider by default, optional CoinGecko overlay for BTC and Finnhub overlay for SPY/GLD
- Local PWA manifest, service worker, and SVG icon
- No CDN, remote font, analytics, or tracking dependency
- Dockerfiles for backend and frontend
- Docker Compose for local full-stack runs
- CI for backend tests, frontend checks/build, and Docker image builds

## Roadmap

### MVP

- [x] Virtual portfolio with $10,000 starting balance
- [x] Local IndexedDB portfolio and transaction store
- [x] Simulated buy/sell flow for stocks, ETFs, crypto, gold, and event markets
- [x] Server-side mock price provider and cache shape
- [x] Optional CoinGecko crypto provider behind the server cache
- [x] Opt-in device-scoped portfolio sync
- [x] Dockerized MVP smoke-tested through the frontend proxy
- [x] External Finnhub stock/ETF/commodity provider behind the server cache
- [ ] Polymarket CLOB read-only market integration
- [ ] Leaderboard with opt-in sync
- [ ] Optional accounts with privacy-preserving defaults
- [ ] Installable PWA with offline portfolio view
- [ ] Dark trading dashboard across desktop and mobile

### Later

- [ ] Seasons with optional resets
- [ ] Private group leaderboards
- [ ] Watchlists and alerts
- [ ] Simulated limit and stop-loss orders
- [ ] Short selling toggle, disabled by default
- [ ] Stats, badges, and export tools
- [ ] Public API for third-party clients

## Architecture Direction

```text
Browser
+-- Svelte app
+-- IndexedDB portfolio store
+-- Optional sync queue
        |
        v
Go API
+-- SQLite
+-- Market data service
+-- Mock provider
+-- Optional CoinGecko provider
+-- Quote cache
+-- Optional account sync
+-- Leaderboard snapshots
        |
        v
External market APIs
+-- Polymarket CLOB
+-- Finnhub
+-- CoinGecko
```

The server owns all external API traffic. Clients should never call market-data providers directly, which keeps API keys private, reduces rate-limit pressure, and avoids leaking user behavior to third parties.

## Environment

Copy `.env.example` to `.env` before running Docker Compose:

```bash
cp .env.example .env
```

For the current foundation, API keys are optional. Market-data features will require provider keys later.

Market-data configuration:

```bash
MARKET_DATA_PROVIDER=mock
MARKET_DATA_CACHE_SECONDS=60
MARKET_DATA_POLL_SECONDS=60
MARKET_DATA_HTTP_TIMEOUT_SECONDS=5
COINGECKO_API_KEY=
FINNHUB_API_KEY=
```

Use `MARKET_DATA_PROVIDER=live` to overlay BTC through CoinGecko and SPY/GLD through Finnhub while keeping unsupported assets on mock fallback. Provider keys are sent only from the server. See [docs/market-data.md](docs/market-data.md).

## Development

```bash
# Backend
make dev-backend

# Frontend
make dev-frontend

# Tests and builds
make ci

# Full stack with Docker
make docker-up
```

Backend defaults to `http://127.0.0.1:8080` during local development. Frontend dev server defaults to `http://127.0.0.1:5173` and proxies `/api` to the backend. Docker Compose exposes the full app at `http://127.0.0.1:3000` and the backend health endpoint at `http://127.0.0.1:18080/healthz`.

Useful API endpoints:

- `GET /healthz`
- `GET /api/config`
- `GET /api/markets`
- `GET /api/quotes?ids=crypto:btc,etf:spy`
- `PUT /api/sync/portfolio`
- `GET /api/sync/portfolio?id=local-default`

Portfolio sync is opt-in and device-scoped for the MVP. The browser sends `X-Koala-Client-ID`, generated and stored in IndexedDB. See [docs/sync.md](docs/sync.md).

## Privacy Principles

- No CDN assets
- No analytics by default
- No remote fonts
- No third-party client SDKs for market data
- Server-side API key handling only
- Optional accounts and opt-in leaderboard sync
- Local-first portfolio state as the product baseline
- Device-scoped sync before account-based sync

## License

MIT, see [LICENSE](LICENSE).
