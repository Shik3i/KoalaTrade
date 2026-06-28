# KoalaTrade

Privacy-first paper trading for event markets, stocks, ETFs, crypto, and gold. KoalaTrade is a no-real-money trading playground: users start with virtual cash, build a portfolio, and can later opt in to sync and leaderboards.

The repository is currently in foundation stage: backend, frontend, Docker, and CI are in place; trading, sync, and market-data features are tracked as roadmap items below.

## Tech Stack

| Component | Choice |
|---|---|
| Frontend | Svelte 5 + Vite + TypeScript SPA/PWA |
| Styling | Local CSS variables and system fonts, no CDN assets |
| Icons | Local bundled `@lucide/svelte` package plus local SVG app icon |
| Backend | Go 1.26 + Chi Router |
| Database | SQLite with pure-Go driver, WAL enabled |
| Client storage | IndexedDB planned |
| Auth | Optional account sync planned, cookie-based sessions preferred |
| Market data | Finnhub, CoinGecko, and Polymarket CLOB planned through server cache |
| Hosting | Hetzner VPS + Caddy + Docker/Compose planned |

## Current Foundation

- Go API server with `/healthz` and `/api/config`
- SQLite initialization with WAL, foreign keys, and busy timeout
- Svelte dashboard shell with local-first/privacy status
- Local PWA manifest, service worker, and SVG icon
- No CDN, remote font, analytics, or tracking dependency
- Dockerfiles for backend and frontend
- Docker Compose for local full-stack runs
- CI for backend tests, frontend checks/build, and Docker image builds

## Roadmap

### MVP

- [ ] Virtual portfolio with $10,000 starting balance
- [ ] Local IndexedDB portfolio and transaction store
- [ ] Simulated buy/sell flow for stocks, ETFs, crypto, gold, and event markets
- [ ] Server-side price cache for external market APIs
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
+-- Price cache
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

## Privacy Principles

- No CDN assets
- No analytics by default
- No remote fonts
- No third-party client SDKs for market data
- Server-side API key handling only
- Optional accounts and opt-in leaderboard sync
- Local-first portfolio state as the product baseline

## License

MIT, see [LICENSE](LICENSE).
