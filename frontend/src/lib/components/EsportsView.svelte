<script lang="ts">
  import { Radio, RefreshCw, Star, Ticket, Trophy } from '@lucide/svelte';
  import type { EsportsMatch, EsportsTeam } from '../api';
  import { matchesLeague } from '../preferences';
  import { formatMoney, type Position } from '../portfolio';
  import InfoTip from './InfoTip.svelte';

  export let matches: EsportsMatch[] = [];
  export let loading = false;
  export let error = '';
  export let cashCents = 0;
  export let positions: Position[] = [];
  export let favoriteTeams: string[] = [];
  export let selectedLeagues: string[] = [];
  export let leagueOptions: string[] = [];
  export let onBet: (match: EsportsMatch, team: EsportsTeam, contracts: number) => void;
  export let onSell: (assetId: string, quantity: number) => void;
  export let onBuyMore: (assetId: string, contracts: number) => void;
  export let onToggleFavorite: (code: string) => void;
  export let onToggleLeague: (id: string) => void;
  export let onRefreshOdds: (matchId: string) => Promise<void>;

  const ORDER_FEE_BPS = 8;
  let stakes: Record<string, number> = {};
  let manageQty: Record<string, number> = {};
  let showOnlyFavorites = false;

  function manageFor(assetId: string, max: number) {
    const value = manageQty[assetId];
    return value && value > 0 ? Math.min(value, max) : max;
  }
  let pending: { matchId: string; teamCode: string } | null = null;
  let refreshingId: string | null = null;

  function stakeFor(id: string) {
    return stakes[id] ?? 10;
  }

  function costCents(priceCents: number, contracts: number) {
    const gross = Math.round(contracts * priceCents);
    return gross + Math.round((gross * ORDER_FEE_BPS) / 10_000);
  }

  function canAfford(priceCents: number, contracts: number) {
    return contracts > 0 && priceCents > 0 && costCents(priceCents, contracts) <= cashCents;
  }

  $: openBets = positions.filter((position) => position.assetId.startsWith('event:lol:'));

  $: filteredMatches = matches.filter((match) => {
    if (match.team1.code === 'TBD' || match.team2.code === 'TBD') return false;
    const isFavorite = favoriteTeams.includes(match.team1.code) || favoriteTeams.includes(match.team2.code);
    if (showOnlyFavorites) return isFavorite;
    const inLeague = selectedLeagues.length === 0 || selectedLeagues.some((league) => matchesLeague(match.league, league));
    return inLeague || isFavorite;
  });

  function timeLabel(iso: string, state: string) {
    if (state === 'inProgress') return 'LIVE';
    const diffMs = new Date(iso).getTime() - Date.now();
    if (diffMs <= 0) return 'startet gleich';
    const hours = Math.floor(diffMs / 3_600_000);
    if (hours < 1) return `in ${Math.max(1, Math.round(diffMs / 60_000))} min`;
    if (hours < 24) return `in ${hours} h`;
    return new Intl.DateTimeFormat('de-DE', { weekday: 'short', day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' }).format(new Date(iso));
  }

  // Step 1: open the confirm bar AND force-refresh this match's Polymarket odds.
  async function startBet(match: EsportsMatch, team: EsportsTeam) {
    pending = { matchId: match.id, teamCode: team.code };
    refreshingId = match.id;
    try {
      await onRefreshOdds(match.id);
    } finally {
      refreshingId = null;
    }
  }

  function pendingTeam(match: EsportsMatch): EsportsTeam | null {
    if (!pending || pending.matchId !== match.id) return null;
    return pending.teamCode === match.team1.code ? match.team1 : match.team2;
  }

  // Step 2: confirm with the freshly refreshed price.
  function confirmBet(match: EsportsMatch) {
    const team = pendingTeam(match);
    if (!team) return;
    const contracts = stakeFor(match.id);
    if (canAfford(team.priceCents, contracts)) {
      onBet(match, team, contracts);
      pending = null;
    }
  }

  function cancelBet() {
    pending = null;
  }
</script>

<div class="esports">
  {#if openBets.length > 0}
    <section class="panel">
      <div class="panel-head"><div><p class="eyebrow">Aktiv</p><h2>Deine Wetten</h2></div><Ticket size={18} /></div>
      <div class="bets-list">
        {#each openBets as bet}
          {@const valueCents = Math.round(bet.quantity * bet.lastPriceCents)}
          {@const pnl = valueCents - Math.round(bet.quantity * bet.averageCostCents)}
          {@const qty = manageFor(bet.assetId, bet.quantity)}
          <div class="bet-row">
            <div class="bet-id"><strong>{bet.symbol}</strong><small>{bet.name}</small></div>
            <div class="bet-num"><span>{bet.quantity} × {formatMoney(bet.lastPriceCents)}</span><small>Ø {formatMoney(bet.averageCostCents)}</small></div>
            <em class={pnl > 0 ? 'up' : pnl < 0 ? 'down' : ''}>{pnl >= 0 ? '+' : '−'}{formatMoney(Math.abs(pnl))}</em>
            <div class="bet-actions">
              <input
                type="number"
                min="1"
                step="1"
                max={bet.quantity}
                value={qty}
                aria-label="Menge"
                on:input={(e) => (manageQty[bet.assetId] = Math.max(1, Math.floor(Number(e.currentTarget.value)) || 1))}
              />
              <button type="button" class="sell" on:click={() => onSell(bet.assetId, qty)}>Verkaufen</button>
              <button type="button" class="buy" on:click={() => onBuyMore(bet.assetId, qty)}>Nachkaufen</button>
            </div>
          </div>
        {/each}
      </div>
    </section>
  {/if}

  <section class="panel esports-head">
    <div class="panel-head"><div><p class="eyebrow">Live aus League of Legends</p><h2>eSports Prediction Markets<InfoTip placement="bottom" text="Ein Prediction Market handelt Wahrscheinlichkeiten: Der Yes-Preis (z. B. 62¢) ist die vom Markt eingeschätzte Siegchance in Prozent. Gewinnt das Team, zahlt jeder Kontrakt 100¢ – du machst also 100¢ minus deinem Kaufpreis Gewinn; verliert es, verfällt der Kontrakt wertlos." /></h2></div><Trophy size={18} /></div>
    <p class="esports-sub">Echte Match-Pläne von lolesports + Live-Quoten von Polymarket. Kaufe „Yes"-Kontrakte auf den Sieger — Auszahlung {formatMoney(100)} pro Kontrakt bei Win.</p>
    <div class="filter-bar">
      <div class="league-filter">
        {#each leagueOptions as league}
          <button class:active={selectedLeagues.includes(league)} type="button" on:click={() => onToggleLeague(league)}>{league}</button>
        {/each}
      </div>
      <button class="fav-toggle" class:active={showOnlyFavorites} type="button" on:click={() => (showOnlyFavorites = !showOnlyFavorites)}>
        <Star size={14} fill={showOnlyFavorites ? 'currentColor' : 'none'} /> Nur Favoriten
      </button>
    </div>
  </section>

  {#if loading}
    <div class="match-grid">
      {#each Array(6) as _}<div class="match-card skeleton"></div>{/each}
    </div>
  {:else if error}
    <p class="empty-state">{error}</p>
  {:else if filteredMatches.length === 0}
    <p class="empty-state">Keine Matches für diese Auswahl. Passe Ligen oder Favoriten an.</p>
  {:else}
    <div class="match-grid">
      {#each filteredMatches as match (match.id)}
        {@const confirmTeam = pendingTeam(match)}
        <article class="match-card" class:live={match.state === 'inProgress'} class:pending={!!confirmTeam}>
          <header class="match-top">
            <span class="league">{match.league}{match.bestOf ? ` · BO${match.bestOf}` : ''}</span>
            <span class="when">
              {#if match.state === 'inProgress'}<Radio size={12} />{/if}
              {timeLabel(match.startTime, match.state)}
            </span>
          </header>

          <div class="teams">
            {#each [match.team1, match.team2] as team}
              <div class="team">
                <div class="team-id">
                  <button class="star" class:on={favoriteTeams.includes(team.code)} type="button" title="Favorit" on:click={() => onToggleFavorite(team.code)}>
                    <Star size={14} fill={favoriteTeams.includes(team.code) ? 'currentColor' : 'none'} />
                  </button>
                  {#if team.image}<img src={team.image} alt="" width="28" height="28" loading="lazy" />{:else}<span class="team-fallback">{team.code}</span>{/if}
                  <div><strong>{team.code}</strong><small>{team.name}</small></div>
                </div>
                {#if match.hasOdds && team.priceCents > 0}
                  <button class="bet-btn" type="button" disabled={refreshingId === match.id} on:click={() => startBet(match, team)}>
                    <span class="prob">{Math.round(team.probBps / 100)}%</span>
                    <span class="px">{formatMoney(team.priceCents)}</span>
                  </button>
                {:else}
                  <span class="no-odds">keine Quote</span>
                {/if}
              </div>
            {/each}
          </div>

          {#if confirmTeam}
            <footer class="confirm-bar">
              {#if refreshingId === match.id}
                <span class="refreshing"><RefreshCw size={13} /> Hole aktuelle Quote …</span>
              {:else if confirmTeam.priceCents > 0}
                <div class="confirm-info">
                  <strong>{confirmTeam.code} @ {formatMoney(confirmTeam.priceCents)}</strong>
                  <small>{stakeFor(match.id)} Kontrakte · {formatMoney(costCents(confirmTeam.priceCents, stakeFor(match.id)))}</small>
                </div>
                <div class="confirm-actions">
                  <button class="ghost" type="button" on:click={cancelBet}>Abbrechen</button>
                  <button class="confirm" type="button" disabled={!canAfford(confirmTeam.priceCents, stakeFor(match.id))} on:click={() => confirmBet(match)}>Bestätigen</button>
                </div>
              {:else}
                <span class="refreshing">Keine aktuelle Quote verfügbar.</span>
                <button class="ghost" type="button" on:click={cancelBet}>Schließen</button>
              {/if}
            </footer>
          {:else if match.hasOdds}
            <footer class="match-foot">
              <label>
                <span>Kontrakte</span>
                <input type="number" min="1" step="1" value={stakeFor(match.id)} on:input={(e) => (stakes[match.id] = Math.max(1, Math.floor(Number(e.currentTarget.value)) || 1))} />
              </label>
              <span class="hint">Einsatz ab {formatMoney(costCents(match.team1.priceCents, stakeFor(match.id)))}</span>
            </footer>
          {/if}
        </article>
      {/each}
    </div>
  {/if}
</div>

<style>
  .esports {
    display: grid;
    gap: 0.75rem;
    align-content: start;
  }

  .esports-sub {
    margin: 0 0 0.85rem;
    color: var(--muted);
    font-size: 0.9rem;
  }

  .filter-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    flex-wrap: wrap;
  }

  .league-filter {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
  }

  .league-filter button {
    min-height: 2rem;
    padding: 0 0.8rem;
    border: 1px solid var(--line);
    border-radius: 999px;
    color: var(--muted);
    font-size: 0.8rem;
    background: var(--bg-2);
    transition: 120ms ease;
  }

  .league-filter button:hover {
    color: var(--text);
    border-color: var(--line-2);
  }

  .league-filter button.active {
    color: var(--green);
    border-color: var(--green-soft);
    background: var(--green-soft);
  }

  .fav-toggle {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    min-height: 2rem;
    padding: 0 0.85rem;
    border: 1px solid var(--line);
    border-radius: 999px;
    color: var(--muted);
    font-size: 0.8rem;
    background: var(--bg-2);
    transition: 120ms ease;
  }

  .fav-toggle.active {
    color: var(--amber);
    border-color: rgba(251, 191, 115, 0.4);
    background: rgba(251, 191, 115, 0.12);
  }

  .bets-list {
    display: grid;
    gap: 0.4rem;
  }

  .bet-row {
    display: grid;
    grid-template-columns: minmax(7rem, 1.4fr) minmax(6rem, 0.9fr) minmax(4rem, auto) auto;
    gap: 0.75rem;
    align-items: center;
    padding: 0.55rem 0.7rem;
    border: 1px solid var(--line);
    border-radius: var(--r-sm);
    background: var(--bg-2);
  }

  .bet-actions {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    justify-self: end;
  }

  .bet-actions input {
    width: 3.6rem;
    min-height: 2rem;
    padding: 0 0.45rem;
    border: 1px solid var(--line);
    border-radius: 6px;
    color: var(--text);
    background: var(--panel);
    outline: none;
  }

  .bet-actions button {
    min-height: 2rem;
    padding: 0 0.7rem;
    border: 1px solid var(--line-2);
    border-radius: 6px;
    color: var(--text);
    font-size: 0.8rem;
    background: var(--panel-3);
    transition: 120ms ease;
  }

  .bet-actions .sell:hover {
    border-color: var(--red);
    color: var(--red);
  }

  .bet-actions .buy:hover {
    border-color: var(--green);
    color: var(--green);
  }

  .bet-id {
    display: grid;
    gap: 0.05rem;
    min-width: 0;
  }

  .bet-id small,
  .bet-num small {
    color: var(--muted);
    font-size: 0.7rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .bet-num {
    display: grid;
    gap: 0.05rem;
    text-align: right;
    font-size: 0.85rem;
  }

  .bet-row em {
    font-style: normal;
    justify-self: end;
  }

  .bet-row button {
    min-height: 2rem;
    padding: 0 0.7rem;
    border: 1px solid var(--line-2);
    border-radius: 6px;
    color: var(--text);
    background: var(--panel-3);
    transition: 120ms ease;
  }

  .bet-row button:hover {
    border-color: var(--line-strong);
  }

  .match-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(19rem, 1fr));
    gap: 0.75rem;
  }

  .match-card {
    display: grid;
    gap: 0.7rem;
    padding: 1rem;
    border: 1px solid var(--line);
    border-radius: var(--r);
    background: linear-gradient(180deg, rgba(255, 255, 255, 0.018), transparent), var(--panel);
    box-shadow: var(--shadow);
  }

  .match-card.live {
    border-color: rgba(251, 113, 133, 0.4);
  }

  .match-card.pending {
    border-color: var(--green-soft);
  }

  .match-card.skeleton {
    min-height: 11rem;
    background: linear-gradient(100deg, rgba(255, 255, 255, 0.03) 30%, rgba(255, 255, 255, 0.07) 50%, rgba(255, 255, 255, 0.03) 70%);
    background-size: 200% 100%;
    animation: shimmer 1.3s infinite;
  }

  .match-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.6rem;
    color: var(--muted);
    font-size: 0.76rem;
  }

  .league {
    font-weight: 600;
    letter-spacing: 0.02em;
  }

  .when {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
  }

  .match-card.live .when {
    color: var(--red);
    font-weight: 600;
  }

  .teams {
    display: grid;
    gap: 0.5rem;
  }

  .team {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
  }

  .team-id {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    min-width: 0;
  }

  .star {
    display: grid;
    place-items: center;
    width: 1.6rem;
    height: 1.6rem;
    border: 0;
    border-radius: 6px;
    color: var(--muted);
    background: transparent;
    transition: 120ms ease;
  }

  .star:hover {
    color: var(--amber);
    background: rgba(255, 255, 255, 0.05);
  }

  .star.on {
    color: var(--amber);
  }

  .team-id img {
    border-radius: 6px;
    object-fit: contain;
    background: rgba(255, 255, 255, 0.04);
  }

  .team-fallback {
    display: grid;
    place-items: center;
    width: 28px;
    height: 28px;
    border-radius: 6px;
    color: var(--muted);
    font-size: 0.62rem;
    background: var(--panel-3);
  }

  .team-id div {
    display: grid;
    gap: 0.05rem;
    min-width: 0;
  }

  .team-id strong {
    font-size: 0.95rem;
  }

  .team-id small {
    color: var(--muted);
    font-size: 0.7rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .bet-btn {
    display: grid;
    gap: 0.05rem;
    min-width: 4.6rem;
    padding: 0.4rem 0.6rem;
    border: 1px solid var(--line-2);
    border-radius: var(--r-sm);
    color: var(--text);
    text-align: right;
    background: var(--bg-2);
    transition: 120ms ease;
  }

  .bet-btn:hover:not(:disabled) {
    border-color: var(--green);
    background: var(--green-soft);
  }

  .bet-btn:disabled {
    cursor: progress;
    opacity: 0.6;
  }

  .bet-btn .prob {
    font-size: 0.92rem;
    font-weight: 650;
  }

  .bet-btn .px {
    color: var(--muted);
    font-size: 0.72rem;
  }

  .no-odds {
    color: var(--muted);
    font-size: 0.74rem;
  }

  .match-foot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    padding-top: 0.6rem;
    border-top: 1px solid var(--line);
  }

  .match-foot label {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    color: var(--muted);
    font-size: 0.78rem;
  }

  .match-foot input {
    width: 4.5rem;
    min-height: 2.1rem;
    padding: 0 0.5rem;
    border: 1px solid var(--line);
    border-radius: 6px;
    color: var(--text);
    background: var(--bg-2);
    outline: none;
  }

  .match-foot .hint {
    color: var(--muted);
    font-size: 0.74rem;
  }

  .confirm-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    flex-wrap: wrap;
    padding: 0.6rem 0.7rem;
    border: 1px solid var(--green-soft);
    border-radius: var(--r-sm);
    background: var(--green-soft);
  }

  .refreshing {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    color: var(--muted);
    font-size: 0.82rem;
  }

  .confirm-info {
    display: grid;
    gap: 0.05rem;
  }

  .confirm-info small {
    color: var(--muted);
    font-size: 0.72rem;
  }

  .confirm-actions {
    display: flex;
    gap: 0.4rem;
  }

  .confirm-actions .ghost {
    min-height: 2.1rem;
    padding: 0 0.75rem;
    border: 1px solid var(--line-2);
    border-radius: 6px;
    color: var(--muted);
    background: transparent;
  }

  .confirm-actions .confirm {
    min-height: 2.1rem;
    padding: 0 0.9rem;
    border: 1px solid transparent;
    border-radius: 6px;
    color: #04140d;
    font-weight: 650;
    background: linear-gradient(180deg, #4ade9f, var(--green));
  }

  .confirm-actions .confirm:disabled {
    cursor: not-allowed;
    color: var(--muted);
    background: var(--panel-3);
  }

  @keyframes shimmer {
    to { background-position: -200% 0; }
  }
</style>
