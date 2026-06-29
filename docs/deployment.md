# Deployment

This guide covers a small production-style deployment using the published GHCR
images and the example Caddy reverse proxy.

## Images

Release tags publish two images:

- `ghcr.io/shik3i/koalatrade-backend:<version>`
- `ghcr.io/shik3i/koalatrade-frontend:<version>`

Use a fixed version tag for deployments. Avoid deploying `latest` unless you are
comfortable with unattended upgrades.

## Compose + Caddy

The [`example/`](../example/) directory contains a production-oriented Compose
file and Caddyfile:

- [`example/docker-compose.yml`](../example/docker-compose.yml)
- [`example/Caddyfile`](../example/Caddyfile)

Create a `.env` file for the deployment or pass variables through your hosting
platform:

```bash
KOALATRADE_VERSION=0.1.1
KOALATRADE_DOMAIN=trade.example.com
AUTH_SECRET=replace-with-a-long-random-secret
ADMIN_USERNAME=admin
ADMIN_PASSWORD=replace-with-a-strong-password
MARKET_DATA_PROVIDER=mock
```

Start the stack:

```bash
docker compose --env-file .env -f example/docker-compose.yml up -d
```

Caddy listens on ports `80` and `443`, obtains TLS certificates automatically,
and proxies traffic to the frontend container. The frontend proxies `/api/*` and
`/healthz` to the backend.

## Required Production Settings

Set these before exposing the service:

| Variable | Why |
|---|---|
| `KOALATRADE_DOMAIN` | Public hostname used by Caddy |
| `AUTH_SECRET` | Keeps admin sessions valid across restarts and signs tokens |
| `ADMIN_PASSWORD` | Enables the admin area with a real password |

Optional live-data settings:

| Variable | Why |
|---|---|
| `MARKET_DATA_PROVIDER` | `mock`, `coingecko`, `finnhub`, or `live` |
| `COINGECKO_API_KEY` | Crypto live-data API key |
| `FINNHUB_API_KEY` | Stocks/ETF/commodity live-data API key |
| `POLYMARKET_API_KEY` | Optional Polymarket API key |

## Data And Backups

The backend stores SQLite data in the `koalatrade_data` Docker volume at
`/data/koalatrade.db`.

Create a backup:

```bash
docker compose --env-file .env -f example/docker-compose.yml exec backend sh -c 'cp /data/koalatrade.db /data/koalatrade.db.backup'
docker cp "$(docker compose --env-file .env -f example/docker-compose.yml ps -q backend)":/data/koalatrade.db.backup ./koalatrade.db.backup
```

For a quiet backup, stop writes first or run during a maintenance window.

## Updating

1. Read [`CHANGELOG.md`](../CHANGELOG.md).
2. Set `KOALATRADE_VERSION` to the new release tag.
3. Pull and restart:

```bash
docker compose --env-file .env -f example/docker-compose.yml pull
docker compose --env-file .env -f example/docker-compose.yml up -d
```

## Smoke Test

After deploy or update:

```bash
curl -f https://trade.example.com/healthz
curl -f https://trade.example.com/api/config
```

Then open the site, place a small paper trade, and confirm the portfolio updates.
