# Configuration

All configuration is via environment variables. Copy [`.env.example`](../.env.example)
to `.env` (git-ignored) and adjust. Docker Compose loads `.env` automatically.

## Server

| Variable | Default | Description |
|---|---|---|
| `APP_NAME` | `KoalaTrade` | App name in `/api/config` |
| `APP_ENV` | `development` | Environment label |
| `PORT` | `8080` | Listen port |
| `DB_PATH` | `data/koalatrade.db` | SQLite path (Docker: `/data/...`) |
| `STARTING_CASH_CENTS` | `1000000` | Virtual starting balance (cents) |

## Market data

Providers are always wired up (Finnhub → Yahoo → CoinGecko → registry fallback); there is no provider-selection switch. **No API keys are required** — the default keyless setup serves equities via Yahoo and crypto via CoinGecko.

| Variable | Default | Description |
|---|---|---|
| `MARKET_DATA_CACHE_SECONDS` | `60` | Quote cache TTL |
| `MARKET_DATA_REFRESH_WINDOW_SECONDS` | `900` | Full window over which all assets are refreshed once (poller staggers evenly to respect per-minute rate limits) |
| `MARKET_DATA_HTTP_TIMEOUT_SECONDS` | `5` | Upstream HTTP timeout |
| `YAHOO_BASE_URL` | — | Yahoo Finance (equities); keyless, base URL override only |
| `COINGECKO_BASE_URL` / `COINGECKO_API_KEY` | — | CoinGecko (crypto); keyless, free Demo key optional (higher rate limit) |
| `FINNHUB_BASE_URL` / `FINNHUB_API_KEY` | — | Finnhub (equities); optional premium override for Yahoo |

## eSports

| Variable | Default | Description |
|---|---|---|
| `LOLESPORTS_API_KEY` | public key | LoL Esports API key (schedule/teams) |
| `LOLESPORTS_BASE_URL` | `https://esports-api.lolesports.com` | LoL Esports base URL |
| `POLYMARKET_BASE_URL` | `https://gamma-api.polymarket.com` | Polymarket Gamma API base URL |
| `ESPORTS_CACHE_SECONDS` | `300` | Schedule/odds/teams cache TTL |

## Admin & auth

| Variable | Default | Description |
|---|---|---|
| `ADMIN_USERNAME` | `admin` | Seeded admin username |
| `ADMIN_PASSWORD` | _empty_ | Seeded admin password (set once on an empty DB). **Empty disables the admin area.** |
| `AUTH_SECRET` | random | HMAC secret for admin tokens. If empty, a random secret is generated at startup, so sessions reset on restart — **set it in production.** Generate with `openssl rand -base64 32`. |

## Deployment notes

- Set `ADMIN_PASSWORD` and `AUTH_SECRET` for any real deployment, and serve over HTTPS.
- Allow outbound HTTPS to `esports-api.lolesports.com` and `gamma-api.polymarket.com` (and Finnhub/CoinGecko if used).
- Never commit `.env` or real keys.
