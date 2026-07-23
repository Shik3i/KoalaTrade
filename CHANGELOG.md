# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres
to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.7.8] - 2026-07-23

### Fixed

- Added safe optional chaining for eSports team code access (`match?.team1?.code`, `match?.team2?.code`) to prevent render errors on malformed payloads.
- Replaced global window stashing for `visibilitychange` event listener in `App.svelte` with component-scoped closure cleanup.

## [0.7.7] - 2026-07-23

### Added

- Added Privacy Policy (`docs/privacy.md`) tailored to KoalaTrade's local-first storage, opt-in sync, PBKDF2/HttpOnly session auth, and server-side API proxying.
- Added interactive Privacy Policy modal to the web interface with German and English translations.
- Added Legal Notice / Imprint link in the site footer pointing to `https://koalastuff.net/legal`.

## [0.7.6] - 2026-07-22

### Added

- The last complete eSports match and odds snapshot is persisted in SQLite and restored before any upstream refresh, so container restarts can serve the page immediately.
- When `AUTH_SECRET` is omitted, a generated 256-bit session-signing key is persisted in SQLite and reused across single-instance restarts.

### Changed

- The permanent header now contains only connection status, language, and one state-aware account button. Shortcuts, anonymous reset, and role-gated administration live in the profile instead.
- Admin requests now use the existing HttpOnly account session. The duplicate admin login form and browser-stored admin bearer token were removed.
- eSports schedule and odds refreshes run in the background while readers continue receiving the last good snapshot.

### Fixed

- Probability bars now stretch their SVG coordinate system across the full card width and render at a more legible height, eliminating the large empty side gutters.
- Docker Compose now enables production cookie behavior with `APP_ENV=production`.

## [0.7.5] - 2026-07-22

### Fixed

- eSports moneyline quotes are now accepted only when both outcomes form one unambiguous two-team pair. A uniquely identified outcome assigns the remaining price to the opposing team; ambiguous, malformed, or incoherent pairs stay unavailable instead of exposing a fake `0%` side.
- Paired Polymarket values are normalized into complementary probabilities and integer-cent paper prices, so both displayed chances and executable prices total exactly `100%` / `$1.00`.
- The two-colour probability bar now uses valid SVG fills and renders visibly instead of appearing black.
- eSports order cards no longer calculate a misleading team-1 "stake" before a team is selected. The confirmation now shows price per contract, quantity, fee, total cost, maximum payout, and possible profit, starting from one contract.
- Client-side fee previews and offline fills now use the same integer rounding as the server, preventing one-cent discrepancies.

## [0.4.1] - 2026-07-18

### Security

- Session cookies are now marked `Secure` in production (`APP_ENV=production`), so browsers only send them over HTTPS. Behind a TLS-terminating reverse proxy the app sees plain HTTP and cannot rely on `r.TLS`, so the flag is gated on the environment.

### Changed

- The embedded frontend now sends caching headers: content-hashed `assets/*` are served `public, max-age=31536000, immutable`, while `index.html` and client-side routes are `no-cache` so a new deploy's asset references are always picked up.
- ROADMAP updated to reflect shipped work (server-computed leaderboard, server-side Limit/Stop engine, deployment guide, production fail-fast, compose healthcheck).

## [0.4.0] - 2026-07-18

### Changed

- **Single Docker image.** The backend now embeds and serves the built frontend from the same origin, replacing the separate `koalatrade-backend` and `koalatrade-frontend` images (and the standalone nginx) with one image: `ghcr.io/shik3i/koalatrade`. The Go server serves the SPA (with client-side-route fallback to `index.html`) for all non-`/api`, non-`/healthz` requests, applying the SPA's `Content-Security-Policy` to HTML/asset responses while the API keeps its strict `default-src 'none'` policy. Deployments now run a single `app` service; update `docker-compose.yml` and the Caddy `reverse_proxy` target accordingly.

## [0.3.1] - 2026-07-17

### Fixed

- Security headers at the nginx edge: HSTS (`Strict-Transport-Security`) is now sent for the SPA document and static assets, not only for `/api/` responses. v0.3.0 added CSP/HSTS on the backend, which only reached the client on proxied `/api/` responses, leaving the main document without HSTS.
- The backend's `Content-Security-Policy` and `Strict-Transport-Security` are now hidden on proxied `/api/` and `/healthz` responses (via `proxy_hide_header`), matching how the other backend security headers were already handled — nginx owns the edge headers, so `/api/` no longer carries duplicate/conflicting CSP headers.

## [0.3.0] - 2026-07-17

### Added

- Server-authoritative competitive trading: market orders, Limit/Stop orders, and eSports bets are executed, priced, and validated server-side (`POST /api/orders`, `POST /api/esports/bet`) at the server's own quote, so prices, cash, and positions can no longer be fabricated by a client.
- Server-side Limit/Stop open-order engine (`open_orders` table + background evaluator, `GET`/`DELETE /api/open-orders`) that fills pending orders at the server price when their trigger is met — even when the browser is closed.
- Background bet settler that pays out resolved eSports "Yes" contracts (100¢ win / 0¢ loss) for every holder, so competition equity stays correct while owners are offline.
- Competitive leaderboard (`GET /api/leaderboard`, new "Rangliste" view) ranking accounts by server-valued equity; anonymous practice portfolios are excluded.
- UX restructure (design "1c"): persistent left icon rail; a real Market/Limit/Stop order ticket with distinct trigger fields and an open-orders queue; a dismissible inline onboarding banner; amber "no feed" and stale-price (⚠) indicators; and a redesigned eSports match card (VS badge, gradient team roundels, two-colour win-probability bar, greyscale no-odds state).
- Admin per-team Polymarket slug management: a per-match view showing which team has a mapping (✓/✗) with inline add/update/remove, plus an "Alle Ligen" toggle to show every match.
- Per-IP request rate limiting on the public API, and Content-Security-Policy + HSTS response headers.

### Changed

- Quote read path is DB-only: `/api/quotes` serves the latest stored quote and never calls a provider, so a burst of requests can no longer stampede the provider, saturate the rate limiter, or blow the request deadline (the staggered poller remains the sole live fetcher).
- Ranked (authenticated) portfolios are fully server-authoritative: `PUT /api/sync/portfolio` ignores client-supplied cash/positions/transactions for a logged-in account and returns the server's own portfolio. A ranked run therefore starts fresh at the starting cash; anonymous practice sync is unchanged.
- `ADMIN_PASSWORD` is now required (fail-fast) in production, matching `AUTH_SECRET`.

### Fixed

- `/api/quotes` no longer hangs and returns 502 under a cold or stale cache (the old read-path fell through to a synchronous live fetch of every stale symbol, which starved the poller).

### Security

- Prices, cash, positions, Limit/Stop triggers, eSports bets, and portfolio sync are all server-authoritative for ranked accounts, so a client can no longer manipulate competition standings.

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

[Unreleased]: https://github.com/Shik3i/KoalaTrade/compare/v0.7.8...HEAD
[0.7.8]: https://github.com/Shik3i/KoalaTrade/compare/v0.7.7...v0.7.8
[0.7.7]: https://github.com/Shik3i/KoalaTrade/compare/v0.7.6...v0.7.7
[0.7.6]: https://github.com/Shik3i/KoalaTrade/compare/v0.7.5...v0.7.6
[0.7.5]: https://github.com/Shik3i/KoalaTrade/compare/v0.7.4...v0.7.5
[0.1.2]: https://github.com/Shik3i/KoalaTrade/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/Shik3i/KoalaTrade/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/Shik3i/KoalaTrade/releases/tag/v0.1.0
