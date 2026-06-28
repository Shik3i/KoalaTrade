# KoalaTrade 🐨📈

> Paper Trading — Bet on events via Polymarket, trade stocks/ETFs/crypto/gold. No real money, just glory.

Start with **$10,000** virtual cash and compete against friends on the leaderboard.
Uses **Polymarket CLOB API** for event betting and **Finnhub + CoinGecko** for real-time market data.

---

## Tech Stack

| Component | Choice |
|---|---|
| **Frontend** | Svelte 5 + Vite SPA + PWA |
| **CSS** | Tailwind CSS |
| **Icons** | Phosphor Icons |
| **Backend** | Go + Chi Router + sqlx |
| **Database** | SQLite (server), IndexedDB (client) |
| **Auth** | JWT (stateless) |
| **Market Data** | Finnhub (stocks/ETFs/gold), CoinGecko (crypto), Polymarket CLOB |
| **Hosting** | Hetzner VPS + Caddy + Docker/Dockge |

## Features

### MVP
- [x] **Virtual Portfolio** — $10,000 starting balance, portfolio overview
- [x] **Polymarket Bets** — Buy/sell Yes/No shares at CLOB API prices
- [x] **Securities** — Stocks, ETFs, Gold, Crypto via Finnhub + CoinGecko
- [x] **Live Prices** — Server-side price cache (updates every 1-2 min)
- [x] **Leaderboard** — Total Worth + % Growth (day/week/month/year)
- [x] **Local-First** — IndexedDB, works offline
- [x] **Optional Account** — Username + password for sync & leaderboard
- [x] **PWA** — Installable (service worker + manifest)
- [x] **Dark Theme** — Default

### Future / Nice-to-Have
- [ ] **Seasons** — Optional 3-month reset with starting capital
- [ ] **Private Group Leaderboards**
- [ ] **Short Selling** (prepared, initially disabled)
- [ ] **Order Types** — Limit/Stop-Loss (simulated)
- [ ] **Watchlist**
- [ ] **Stats & Badges**
- [ ] **Public API** for third-party clients

## Architecture

### Local-First + Sync

```
Browser (IndexedDB)
+-- portfolio        — Current positions
+-- transactions     — Trade history
+-- watchlist        — Tracked markets
+-- user_profile     — Username (if registered)
        |
        v  Sync on registration
Server (Go + SQLite)
+-- users            — Username + bcrypt hash
+-- transactions     — Synced trade copy
+-- leaderboard      — Portfolio values + growth rates
+-- price_cache      — Cached price data
```

### Price Update Strategy

Server polls prices in background (goroutine + ticker):

| Asset | Source | Interval |
|---|---|---|
| Stocks / ETFs | Finnhub (60 calls/min) | ~1-2 min per symbol |
| Crypto | CoinGecko (30 calls/min, no key) | Every 1 min |
| Gold / Commodities | Finnhub | Part of rotation |
| Polymarket Markets | CLOB API (unlimited) | On-demand + cache |

All clients fetch prices from the server cache — one API call serves 100 users.

## Project Structure

```
koalatrade/
+-- .env.example              # Environment variable template
+-- Dockerfile.backend        # Go backend container
+-- Dockerfile.frontend       # Svelte frontend container
+-- LICENSE                   # MIT
+-- Makefile                  # Development commands
+-- README.md
+-- docker-compose.yml
+-- .github/
|   +-- dependabot.yml
|   +-- workflows/
|       +-- docker-release.yml # Build & push on v* tags only
+-- backend/                  # Go API server
|   +-- cmd/server/main.go
|   +-- internal/
|   |   +-- handler/         # HTTP routes
|   |   +-- model/           # Data models
|   |   +-- repository/      # SQLite access
|   |   +-- service/         # Business logic + price fetcher
|   +-- go.mod
+-- docs/                     # Documentation
+-- frontend/                 # Svelte 5 SPA + PWA
|   +-- src/
|   |   +-- routes/          # SPA routes
|   |   +-- lib/
|   |   |   +-- components/  # UI components
|   |   |   +-- stores/      # IndexedDB/LocalStorage
|   |   +-- app.html
|   |   +-- service-worker.js
|   +-- static/
|   |   +-- manifest.json
|   +-- package.json
|   +-- svelte.config.js
```

## Environment

Copy `.env.example` to `.env` and fill in your API keys:

```bash
cp .env.example .env
```

## Development

```bash
# Start backend (Go)
make dev-backend

# Start frontend (Svelte)
make dev-frontend

# Full stack with Docker
make docker-up
```

## License

MIT — see [LICENSE](LICENSE).
