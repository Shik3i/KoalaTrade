# eSports Prediction Markets

KoalaTrade turns real League of Legends matches into **prediction markets**: you
buy "Yes" contracts on a team to win, settled through the same paper portfolio as
every other asset (no separate betting system).

## Data flow

1. **Team catalogue and logos** — once per week on Monday at 03:15 UTC, the
   server fetches `getTeams` from lolesports, stores team names, codes, league,
   logo bytes, and content types in the SQLite `esports_teams` table. The API
   exposes logos only through same-origin `/api/esports/teams/{code}/logo`.
2. **Schedule** — `GET /api/esports/matches` fetches the lolesports schedule
   (`esports-api.lolesports.com`, `LOLESPORTS_API_KEY`) and returns upcoming /
   in-progress matches with local team-logo URLs.
3. **Match details** — the match card loads `getEventDetails` only when
   expanded. Game IDs, game states, series scores, and external stream/VOD
   links are cached in SQLite and rendered as normal external links.
4. **Odds** — for each match the server guesses Polymarket event slugs
   (`lol-<team>-<team>-<YYYY-MM-DD>`, both orders, ±2 days for timezone skew) and
   reads the **moneyline / "Match Winner"** market from `gamma-api.polymarket.com`.
   The implied probability becomes the Yes price in cents (e.g. 0.865 → 87¢).
5. **Trading** — buying creates a portfolio position with asset id
   `event:lol:<matchId>:<teamCode>` at the Yes price; payout is $1.00/contract on a win.
6. **Resolution** — completed results are captured from the schedule using
   `result.outcome` or the completed game-score fallback. If a position has no
   stored result yet, the server lazily checks `getEventDetails` and derives the
   winner from completed game scores before persisting it. Results are served via
   `GET /api/esports/results`. The client auto-settles open bets: winning Yes →
   100¢, losing Yes → 0¢, credited automatically.

The schedule is cached for `ESPORTS_CACHE_SECONDS`; a background poller keeps
the schedule and results fresh. Team metadata and logos use the persisted
weekly snapshot, with a stale-snapshot refresh at startup. Polymarket has no
rate limit, so odds are force-refreshed on demand right before a bet
(`GET /api/esports/matches/{id}/odds`).

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
- **Matches without odds** — a diagnostic list with team names; clicking a team prefills a mapping.
- **Slug diagnostics** — preview generated Polymarket slug candidates and live-test whether a pending mapping resolves to a moneyline event.
- **Status & cache** — schedule age, match/odds/results counts; **Force-Refresh**.

Endpoints (all token-gated except login):

| Method | Path |
|---|---|
| POST | `/api/auth/login` |
| GET | `/api/admin/mappings` |
| PUT | `/api/admin/mappings` |
| DELETE | `/api/admin/mappings/{code}` |
| POST | `/api/admin/slug-preview` |
| GET | `/api/admin/status` |
| POST | `/api/admin/refresh` |

## Limitations (v0.1.0)

- Only matches Polymarket actually lists get odds — smaller leagues often show "no odds".
- Auto-resolution still depends on at least one reachable LoL API endpoint; schedule
  results and lazy match-details results are persisted to mitigate missed polls.
- eSports positions re-price when the eSports tab is opened (not on the 30s quote poll). See the [Roadmap](../ROADMAP.md).
