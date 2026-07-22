<div align="center">

# 🐨 KoalaTrade

**A modern, privacy-first paper-trading desk for stocks, ETFs, crypto, commodities — and live eSports prediction markets.**

Trade with virtual cash, learn the markets, and compete — no real money, no tracking.

[![CI](https://github.com/Shik3i/KoalaTrade/actions/workflows/ci.yml/badge.svg)](https://github.com/Shik3i/KoalaTrade/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Roadmap](https://img.shields.io/badge/docs-roadmap-blue.svg)](ROADMAP.md)

</div>

---

## Highlights

- **Trading desk** — watchlist, an interactive price chart (SMA + crosshair), a simulated order book with depth, and an order ticket (market / limit / stop) with quantity presets and keyboard shortcuts.
- **Portfolio analytics** — equity curve, realized vs. unrealized P&L, drawdown, positions, and order history.
- **eSports prediction markets** — real League of Legends schedules from lolesports with live "match winner" odds from Polymarket, traded as Yes-contracts through the paper portfolio. Bets auto-resolve when a match completes; sell or top up anytime at the current price.
- **Profile** — favorite teams and default leagues (stored locally), coupled with the eSports page filter.
- **Admin area** — role-gated through the normal account session, with Polymarket team-code mappings, cache status, and force-refresh.
- **Privacy-first** — no account required to trade, portfolio lives in your browser (IndexedDB), no CDN/fonts/analytics/trackers. The server owns all third-party API traffic so keys stay private.

> Status: **MVP (v0.2.0)**. See the [Roadmap](ROADMAP.md) for what's next.

## Tech Stack

| Component | Choice |
|---|---|
| Frontend | Svelte 5 + Vite + TypeScript SPA/PWA |
| Charts/UI | Hand-built SVG, local CSS variables, `@lucide/svelte` icons — no CDN |
| Backend | Go 1.26 + chi router |
| Database | SQLite (pure-Go `modernc.org/sqlite`, WAL) |
| Client storage | IndexedDB (portfolio, preferences, device id) |
| Account auth | PBKDF2 password hashing + HMAC-signed HttpOnly sessions (stdlib only) |
| Market data | Mock by default; optional CoinGecko (crypto) + Finnhub (stocks/ETF/commodity) |
| eSports data | lolesports schedule/teams + locally served team logos + Polymarket odds (server-side) |
| Packaging | Docker + Docker Compose; images published to GHCR on tag |

## Quick Start

### Docker (recommended)

```bash
cp .env.example .env   # then edit values (see Configuration)
docker compose up --build
```

- Frontend: http://localhost:3000
- Backend health (direct): http://127.0.0.1:18080/healthz — the frontend proxies `/api/*` to the backend.

### Local development

```bash
# Terminal 1 — backend (http://127.0.0.1:8080)
cd backend && go run ./cmd/server

# Terminal 2 — frontend (http://127.0.0.1:5173, proxies /api to the backend)
cd frontend && npm install && npm run dev
```

The dev proxy target is configurable: set `KOALA_API_TARGET` (default `http://127.0.0.1:8080`) before `npm run dev` to point at a backend on another port. A `Makefile` provides shortcuts (`make dev-backend`, `make dev-frontend`, `make ci`, `make docker-up`).

## Configuration

All configuration is via environment variables — see [`.env.example`](.env.example) and [docs/configuration.md](docs/configuration.md). The essentials:

| Variable | Default | Purpose |
|---|---|---|
| `STARTING_CASH_CENTS` | `1000000` | Virtual starting balance ($10,000) |
| `MARKET_DATA_PROVIDER` | `mock` | `mock`, `coingecko`, `finnhub`, or `live` |
| `MARKET_DATA_REFRESH_WINDOW_SECONDS` | `900` | Window over which quote refreshes are staggered to respect rate limits |
| `FINNHUB_API_KEY` / `COINGECKO_API_KEY` | — | Optional live market-data overlays |
| `LOLESPORTS_API_KEY` | public key | LoL Esports schedule/teams |
| `ESPORTS_CACHE_SECONDS` | `300` | eSports schedule/odds cache TTL |
| `ADMIN_USERNAME` / `ADMIN_PASSWORD` | `admin` / _empty_ | Seeds the admin user once; empty password disables admin |
| `AUTH_SECRET` | persisted in SQLite | Overrides the local session-signing key; set one shared value for multiple replicas |

> ⚠️ `.env` is git-ignored — never commit real keys. Set `ADMIN_PASSWORD` for the initial admin account. A single instance persists its generated session key in SQLite; multiple replicas must share `AUTH_SECRET`. The backend also needs outbound access to `esports-api.lolesports.com` and `gamma-api.polymarket.com`.

## API Overview

| Method | Path | Description |
|---|---|---|
| GET | `/healthz` | Health check |
| GET | `/api/config` | Public config |
| GET | `/api/markets` | All markets |
| GET | `/api/markets/{assetId}/history?range=1H..1Y` | Price history (OHLCV) |
| GET | `/api/quotes?ids=` | Current quotes |
| GET | `/api/esports/matches` | LoL matches with Polymarket odds |
| GET | `/api/esports/matches/{id}/details` | On-demand match games, scores, streams, and VOD links |
| GET | `/api/esports/matches/{id}/odds` | On-demand odds refresh for one match |
| GET | `/api/esports/teams` | Team catalogue |
| GET | `/api/esports/teams/{code}/logo` | Locally stored team logo |
| GET | `/api/esports/results?ids=` | Settled results (bet resolution) |
| GET/PUT | `/api/sync/portfolio` | Opt-in device-scoped portfolio sync |
| POST | `/api/auth/register` | Create user account → HttpOnly session cookie |
| POST | `/api/auth/login` | User login → HttpOnly session cookie |
| POST | `/api/auth/logout` | Clear session cookie |
| GET | `/api/auth/me` | Current authenticated user |
| PATCH | `/api/account/` | Update display name |
| PUT | `/api/account/password` | Change password |
| GET | `/api/account/export` | Export account profile + account-bound portfolios |
| DELETE | `/api/account/portfolio-data` | Delete account-bound portfolio data |
| DELETE | `/api/account/` | Delete account |
| `*` | `/api/admin/*` | Token-gated admin (settings, mappings, slug diagnostics, status, refresh) |

## Documentation

- [docs/architecture.md](docs/architecture.md) — system overview
- [docs/configuration.md](docs/configuration.md) — full environment reference
- [docs/deployment.md](docs/deployment.md) — Docker/GHCR deployment with Caddy
- [docs/esports.md](docs/esports.md) — eSports markets, Polymarket odds, team mappings, admin
- [docs/market-data.md](docs/market-data.md) — market-data providers & caching
- [docs/sync.md](docs/sync.md) — portfolio sync model
- [ROADMAP.md](ROADMAP.md) · [CHANGELOG.md](CHANGELOG.md) · [CONTRIBUTING.md](CONTRIBUTING.md) · [SECURITY.md](SECURITY.md)

## Project Structure

```
KoalaTrade/
├── backend/            Go API (chi, SQLite); internal/{config,server,marketdata,esports,storage,auth}
├── frontend/           Svelte 5 + Vite SPA; src/lib/{components,api,portfolio,preferences,...}
├── docs/               Architecture, configuration, and feature docs
├── example/            Production-style Compose and Caddy examples
├── Dockerfile          docker-compose.yml   (single image: Go backend serves the embedded SPA)
└── .github/            CI, release workflow, dependabot, issue/PR templates
```

## Privacy Principles

No CDN assets · no remote fonts · no analytics by default · no third-party client SDKs for market data · server-side API-key handling only · local-first portfolio state · device-scoped sync before accounts.

## Contributing

Contributions are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md) and the [Code of Conduct](CODE_OF_CONDUCT.md).

## License

[MIT](LICENSE) © Shik3i

> **Disclaimer:** KoalaTrade is for education and entertainment. It uses virtual money only and is **not** financial advice or a real trading/betting platform.
