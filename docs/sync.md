# Portfolio Sync

KoalaTrade sync is opt-in and device-scoped in the MVP.

## Current Model

- The portfolio stays local in IndexedDB by default.
- The browser creates a random `clientId` with `crypto.randomUUID()` and stores it in IndexedDB.
- The `clientId` is sent only when the user clicks sync.
- The server derives an internal portfolio id from `clientId + portfolioId` with SHA-256.
- The server never returns the internal portfolio id.
- Successful sync returns transactions as `synced`, which clears the local queue.

## Endpoints

```http
PUT /api/sync/portfolio
X-Koala-Client-ID: <local-client-id>
Content-Type: application/json
```

```http
GET /api/sync/portfolio?id=local-default
X-Koala-Client-ID: <local-client-id>
```

The sync payload matches the frontend `PortfolioSnapshot` shape.

## Privacy Boundaries

- No email, username, wallet address, or account identity is required.
- No market-data provider receives portfolio, watchlist, transaction, or client id data.
- `clientId` acts as a bearer identifier for this MVP. Treat it as device-local state.
- Multi-device sync should not reuse this model as authentication. Add proper sessions first.

## Validation

The backend rejects:

- Missing or malformed `X-Koala-Client-ID`
- Unknown JSON fields
- Payloads over 1 MiB
- Unsupported schema versions
- Negative cash values
- Invalid asset kinds, sides, statuses, quantities, prices, or timestamps
- More than 200 positions or 1000 transactions
- Assets outside the current server-side market catalog

## Next Upgrade

Add account sessions with `HttpOnly`, `Secure`, `SameSite=Strict` cookies before enabling cross-device sync, leaderboards, or public profile features.
