# 🐨 KoalaTrade Roadmap

This roadmap covers what we want to build **after** merging and releasing **v0.1.0**. It is intentionally living — priorities will shift. Items are grouped by theme; checkboxes track progress.

> Shipped in **v0.1.0**: trading desk (chart, order book, order ticket), portfolio analytics, eSports prediction markets (lolesports + Polymarket) with auto-resolution, profile preferences, admin area (login, team mappings, cache status), staggered market-data polling, Docker/GHCR release pipeline. See [CHANGELOG.md](CHANGELOG.md).

---

## v0.2.0 — Accounts & Auth (highest priority)

Today only the admin user exists; regular users are anonymous (browser-local portfolio + device-scoped sync). The next milestone makes real user accounts a first-class concept.

- [x] **Registration / login workflow** for end users
  - [x] `POST /api/auth/register`, `POST /api/auth/login`, `POST /api/auth/logout`, `GET /api/auth/me`
  - [x] Reuse the existing PBKDF2 + token primitives (`internal/auth`); move tokens to secure, `HttpOnly`, `SameSite` cookies instead of `localStorage`
  - [x] Frontend register/login views + authenticated app state
  - [x] Password rules, rate limiting, and lockout on repeated failures
- [x] **Admin toggle: registration on/off**
  - [x] Setting persisted in `app_meta` (e.g. `registration_open`), exposed via `/api/config` and editable in the admin area
  - [ ] Optional invite-code / allowlist mode
- [x] **Roles & permissions** — `user` vs `admin`; gate admin UI by role from the token
- [x] **Account ⇄ portfolio migration** — bind the current device-local portfolio to an account on first login; resolve conflicts with the existing sync model
- [x] **Account management** — change password, change display name, delete account (GDPR-friendly export/delete)

## v0.3.0 — Admin & Operations

- [ ] **User management** in the admin area — list/search users, disable/enable, reset password, change role, view portfolios
- [ ] **Audit log** for admin actions (mappings, user changes, refreshes)
- [ ] **Feature flags / settings panel** — registration toggle, leaderboard on/off, maintenance mode, starting cash
- [ ] **More eSports admin** — manual match/odds override, mark a match resolved, bulk-import team mappings, mapping "test" that shows which slugs resolve
  - [x] Mapping test with generated Polymarket slug preview
- [ ] **Observability** — structured request logging, `/metrics` (Prometheus), basic dashboards
- [ ] **Backups** — scheduled SQLite backups + documented restore

## v0.4.0 — Competition & Social

- [ ] **Leaderboard** (schema already exists) — opt-in, periodic snapshots, return-based ranking
- [ ] **Seasons** with optional resets and historical archives
- [ ] **Private group leaderboards / leagues** with invite links
- [ ] **Profiles** — public (opt-in) profile pages, badges, achievements
- [ ] **Sharing** — shareable trade/bet cards and portfolio snapshots

## Markets & Trading depth

- [ ] **More eSports titles** beyond LoL (Valorant, CS, Dota) via the same odds pipeline
- [ ] **Real Polymarket coverage expansion** — better slug matching, more leagues, props markets
- [ ] **Functional limit/stop orders** with a server-side matching/trigger engine (currently paper-filled)
- [ ] **Short selling** toggle (off by default)
- [ ] **Watchlists & price alerts**
- [ ] **More history/candles** with real provider candles where available

## Platform & Quality

- [ ] **Installable PWA** with offline portfolio view + background sync
- [ ] **Mobile polish** pass across all views
- [ ] **i18n** — currently mixed German/English; pick a strategy and add a language toggle
- [ ] **Test coverage** — backend handler/integration tests for esports, admin, auth; frontend component tests; an e2e smoke test
- [ ] **Accessibility** audit (focus management, ARIA, keyboard nav)
- [ ] **Public, documented API** + rate limiting for third-party clients
- [ ] **Deployment guide** — Hetzner VPS + Caddy/Traefik + Docker, TLS, secrets management

## Known follow-ups from v0.1.0

- [ ] eSports bet positions only re-price when the eSports tab is opened (not via the 30s quote poll) — unify mark-to-market
- [ ] Auto-resolution depends on results staying in the lolesports window; persisted results mitigate this, but add a longer-term results store / cleanup
- [ ] `AUTH_SECRET` defaults to a random per-start value — document that production must set it (done in `.env.example`); consider failing fast if unset in `production`
- [ ] Add a healthcheck to `docker-compose.yml` and `depends_on: condition: service_healthy`

---

Have an idea? Open a [feature request](https://github.com/Shik3i/KoalaTrade/issues/new/choose).
