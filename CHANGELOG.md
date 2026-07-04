# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres
to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.0] - 2026-07-04

### Added

- Keyless market data: Yahoo Finance serves the 121 stocks/ETFs/commodities (live quotes + historical candles) and CoinGecko serves crypto, so the full 129-asset catalogue works with no API keys. Finnhub is now an optional premium override.
- Continuous, gap-driven history-backfill maintainer that keeps each asset/tier populated (crypto via CoinGecko, equities via Yahoo) and self-heals gaps from downtime.
- Per-provider rate limiters (Yahoo/Finnhub/CoinGecko) so the poller and backfill can never combine to exceed a provider's free-tier limit.
- Real market-detail panel (live quote + your position) replacing the synthetic order book.
- First-run onboarding modal, a reset-portfolio confirmation dialog, and explanatory tooltips (`InfoTip`) for trading jargon (SMA, P&L, Drawdown, fees, prediction-market contracts, …).
- End-user accounts with registration/login/logout/me endpoints, HttpOnly cookie sessions, roles, registration toggle, and account-bound portfolio sync.
- Account management for display names, password changes, account export, portfolio-data deletion, and account deletion.
- eSports admin slug diagnostics with team names, generated Polymarket slug previews, and live mapping tests.

### Changed

- Read endpoints no longer trigger provider fetches; prices are served from SQLite and refreshed exclusively by the staggered poller. Provider chain is now Finnhub → Yahoo → CoinGecko → registry.
- Poll interval is derived from the refresh window; the asset catalogue is the single source of truth (the Finnhub/Yahoo symbol maps are derived from it).
- History retention reworked into downsampling tiers: fine tiers are bounded while the daily tier is kept forever, giving long histories without database bloat.

### Fixed

- Chart history is no longer wiped on every restart (idempotent, column-aware migration).
- eSports bets — including settled 0¢ losing bets — now sync correctly (event assets registered, transaction price constraint relaxed to ≥ 0).
- Null-guard when markets fail to load (no blank screen); portfolio state is assigned before persisting to close a lost-update race.

### Removed

- All simulated/placeholder data: the synthetic order book, seeded sparklines, invented event resolution dates, and the misleading "mock" provider naming/label.
- Dead configuration: `MARKET_DATA_PROVIDER` and `MARKET_DATA_POLL_SECONDS`.

## [0.1.2] - 2026-06-29

### Added

- Deployment documentation and an `example/` Compose + Caddyfile for production-style runs (GHCR images, TLS, backups, updates, smoke tests).
- Container healthchecks for the backend and frontend; dependents now wait for `service_healthy`.

## [0.1.1] - 2026-06-29

Release hardening for the Docker/GHCR pipeline.

### Changed

- CI and Docker release workflows now use Node.js 24 and current major versions of the GitHub/Docker Actions, with `concurrency`, per-job `timeout-minutes`, and least-privilege `permissions`.
- Docker releases now run backend and frontend verification before publishing GHCR images.

## [0.1.0] - 2026-06-29

First MVP release. Published as Docker images to GHCR.

### Added

- **Trading desk**: redesigned dark UI with Trade / Portfolio / Markets / eSports tabs and a Profile and Admin area.
- **Interactive price chart** (`GET /api/markets/{assetId}/history`) with SMA overlay, crosshair, timeframe switching, and skeleton loading; deterministic mock OHLCV anchored to the live price.
- **Simulated order book** with bid/ask depth and spread.
- **Order ticket** with market/limit/stop types, quantity presets, and keyboard shortcuts (`B`/`S`/`1-6`/`?`).
- **Portfolio analytics**: equity curve, realized/unrealized P&L, max drawdown, positions, and order history.
- **eSports prediction markets**: live LoL schedule from lolesports + "match winner" odds from Polymarket, traded as Yes-contracts via the paper portfolio.
  - Two-step bet with on-demand fresh odds (`GET /api/esports/matches/{id}/odds`).
  - Automatic bet resolution from completed results (`GET /api/esports/results`), persisted across restarts.
  - Partial sell / buy-more at the current price.
  - Team catalogue endpoint (`GET /api/esports/teams`).
- **Profile preferences**: favorite teams and default leagues (stored in IndexedDB), coupled with the eSports page filter.
- **Admin area**: admin user seeded once from `ADMIN_USERNAME`/`ADMIN_PASSWORD`; `POST /api/auth/login` issues an HMAC bearer token gating `/api/admin/*`; manage Polymarket team-code mappings, view cache status, and force-refresh. PBKDF2 + HMAC, standard library only.
- **Staggered market-data polling** to stay under free-tier per-minute rate limits (`MARKET_DATA_REFRESH_WINDOW_SECONDS`).
- **Toasts, skeleton loaders, and responsive layout** across the app.
- Project documentation: README, ROADMAP, CONTRIBUTING, SECURITY, CODE_OF_CONDUCT, `docs/`, and issue/PR templates.

### Fixed

- Market history endpoint now URL-decodes the asset id (colon in `crypto:btc`).
- Docker release workflow lowercases the image owner so GHCR pushes succeed.

[Unreleased]: https://github.com/Shik3i/KoalaTrade/compare/v0.1.2...HEAD
[0.1.2]: https://github.com/Shik3i/KoalaTrade/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/Shik3i/KoalaTrade/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/Shik3i/KoalaTrade/releases/tag/v0.1.0
