# eSports Prediction Markets

KoalaTrade turns real League of Legends matches into **prediction markets**: you
buy "Yes" contracts on a team to win, settled through the same paper portfolio as
every other asset (no separate betting system).

## Data flow

1. **Schedule** — `GET /api/esports/matches` fetches the lolesports schedule
   (`esports-api.lolesports.com`, `LOLESPORTS_API_KEY`) and returns upcoming /
   in-progress matches with team names, codes, and logos.
2. **Odds** — for each match the server guesses Polymarket event slugs
   (`lol-<team>-<team>-<YYYY-MM-DD>`, both orders, ±2 days for timezone skew) and
   reads the **moneyline / "Match Winner"** market from `gamma-api.polymarket.com`.
   The implied probability becomes the Yes price in cents (e.g. 0.865 → 87¢).
3. **Trading** — buying creates a portfolio position with asset id
   `event:lol:<matchId>:<teamCode>` at the Yes price; payout is $1.00/contract on a win.
4. **Resolution** — completed results (`result.outcome`) are captured from the
   schedule, persisted, and served via `GET /api/esports/results`. The client
   auto-settles open bets: winning Yes → 100¢, losing Yes → 0¢, credited automatically.

Cached for `ESPORTS_CACHE_SECONDS`; a background poller keeps the schedule and
results fresh. Polymarket has no rate limit, so odds are force-refreshed on
demand right before a bet (`GET /api/esports/matches/{id}/odds`).

## Why team mappings are needed

Polymarket sometimes abbreviates teams differently than lolesports (e.g. lolesports
`EINS` vs Polymarket `ES1`). Slug guessing uses each team's **code and name**, which
covers most teams — but mismatches produce "no odds". Admins fix these with a
**team mapping** (`originalCode → polymarketCode`), which is injected as an extra
slug identifier so the odds resolve.

## Admin area

Open the shield icon in the top bar (requires `ADMIN_PASSWORD` to be set so an
admin user is seeded). After logging in you can:

- **Team mappings** — add/remove lolesports→Polymarket code mappings.
- **Matches without odds** — a diagnostic list; clicking a team prefills a mapping.
- **Status & cache** — schedule age, match/odds/results counts; **Force-Refresh**.

Endpoints (all token-gated except login):

| Method | Path |
|---|---|
| POST | `/api/auth/login` |
| GET | `/api/admin/mappings` |
| PUT | `/api/admin/mappings` |
| DELETE | `/api/admin/mappings/{code}` |
| GET | `/api/admin/status` |
| POST | `/api/admin/refresh` |

## Limitations (v0.1.0)

- Only matches Polymarket actually lists get odds — smaller leagues often show "no odds".
- Auto-resolution relies on results being captured while in the schedule window; results are persisted to mitigate this.
- eSports positions re-price when the eSports tab is opened (not on the 30s quote poll). See the [Roadmap](../ROADMAP.md).
