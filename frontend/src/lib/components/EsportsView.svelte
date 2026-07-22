<script lang="ts">
  import { ChevronDown, ExternalLink, Radio, RefreshCw, Star, Ticket, Trophy } from '@lucide/svelte';
  import { fetchEsportsMatchDetails, type EsportsMatch, type EsportsMatchDetails, type EsportsMatchVideo, type EsportsTeam } from '../api';
  import { matchesLeague } from '../preferences';
  import { formatMoney, type Position } from '../portfolio';
  import InfoTip from './InfoTip.svelte';
  import { get } from 'svelte/store';
  import { t, locale } from '../i18n';

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
  export let onRefreshOdds: (matchId: string) => Promise<boolean>;

  const ORDER_FEE_BPS = 8;
  let stakes: Record<string, number> = {};
  let manageQty: Record<string, number> = {};
  let showOnlyFavorites = false;
  let showAllLeagues = false;

  function manageFor(assetId: string, max: number) {
    const value = manageQty[assetId];
    return value && value > 0 ? Math.min(value, max) : max;
  }
  let pending: { matchId: string; teamCode: string } | null = null;
  let refreshingId: string | null = null;
  let expandedDetailsId: string | null = null;
  let matchDetails: Record<string, EsportsMatchDetails> = {};
  let detailsLoadingId: string | null = null;
  let detailsErrors: Record<string, string> = {};

  function stakeFor(id: string) {
    return stakes[id] ?? 1;
  }

  function grossCents(priceCents: number, contracts: number) {
    return Math.round(contracts * priceCents);
  }

  function feeCents(priceCents: number, contracts: number) {
    return Math.floor((grossCents(priceCents, contracts) * ORDER_FEE_BPS) / 10_000);
  }

  function costCents(priceCents: number, contracts: number) {
    return grossCents(priceCents, contracts) + feeCents(priceCents, contracts);
  }

  function canAfford(priceCents: number, contracts: number) {
    return contracts > 0 && priceCents > 0 && priceCents < 100 && costCents(priceCents, contracts) <= cashCents;
  }

  $: openBets = positions.filter((position) => position.assetId.startsWith('event:lol:'));

  $: filteredMatches = matches.filter((match) => {
    if (match.team1.code === 'TBD' || match.team2.code === 'TBD') return false;
    const isFavorite = favoriteTeams.includes(match.team1.code) || favoriteTeams.includes(match.team2.code);
    if (showOnlyFavorites) return isFavorite;
    if (showAllLeagues) return true;
    const inLeague = selectedLeagues.length === 0 || selectedLeagues.some((league) => matchesLeague(match.league, league));
    return inLeague || isFavorite;
  });

  // `_loc` is unused but passed from the template as $locale so the label
  // re-renders when the language changes.
  function timeLabel(iso: string, state: string, _loc?: string) {
    const tr = get(t);
    if (state === 'inProgress') return tr('esports.live');
    const diffMs = new Date(iso).getTime() - Date.now();
    if (diffMs <= 0) return tr('esports.startingSoon');
    const hours = Math.floor(diffMs / 3_600_000);
    if (hours < 1) return tr('esports.inMin', { min: Math.max(1, Math.round(diffMs / 60_000)) });
    if (hours < 24) return tr('esports.inHours', { hours });
    const dateLocale = get(locale) === 'de' ? 'de-DE' : 'en-US';
    return new Intl.DateTimeFormat(dateLocale, { weekday: 'short', day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' }).format(new Date(iso));
  }

  // Step 1: open the confirm bar AND force-refresh this match's Polymarket odds.
  async function startBet(match: EsportsMatch, team: EsportsTeam) {
    pending = { matchId: match.id, teamCode: team.code };
    refreshingId = match.id;
    try {
      if (!(await onRefreshOdds(match.id))) {
        pending = null;
      }
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

  // Normalise and round as one pair so the two displayed chances always add
  // up to exactly 100%, including half-cent Polymarket prices.
  function displayProbabilities(match: EsportsMatch) {
    const a = match.team1.probBps;
    const b = match.team2.probBps;
    const total = a + b;
    const team1 = total > 0 ? Math.round((a / total) * 100) : 50;
    return { team1, team2: 100 - team1 };
  }

  async function toggleDetails(matchId: string) {
    if (expandedDetailsId === matchId) {
      expandedDetailsId = null;
      return;
    }
    expandedDetailsId = matchId;
    if (matchDetails[matchId] || detailsLoadingId === matchId) return;

    detailsLoadingId = matchId;
    detailsErrors = { ...detailsErrors, [matchId]: '' };
    try {
      matchDetails = { ...matchDetails, [matchId]: await fetchEsportsMatchDetails(matchId) };
    } catch (error) {
      detailsErrors = {
        ...detailsErrors,
        [matchId]: error instanceof Error ? error.message : 'Match details unavailable'
      };
    } finally {
      detailsLoadingId = null;
    }
  }

  function videoLabel(video: EsportsMatchVideo) {
    const tr = get(t);
    return video.kind === 'vod' ? tr('esports.watchVod') : tr('esports.watchStream');
  }
</script>

<div class="esports">
  {#if openBets.length > 0}
    <section class="panel">
      <div class="panel-head"><div><p class="eyebrow">{$t('esports.active')}</p><h2>{$t('esports.yourBets')}</h2></div><Ticket size={18} /></div>
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
                aria-label={$t('esports.qty')}
                title={$t('esports.qtyTitle')}
                on:input={(e) => (manageQty[bet.assetId] = Math.max(1, Math.floor(Number(e.currentTarget.value)) || 1))}
              />
              <button type="button" class="sell" title={$t('esports.sellTitle')} on:click={() => onSell(bet.assetId, qty)}>{$t('side.sellVerb')}</button>
              <button type="button" class="buy" title={$t('esports.buyMoreTitle')} on:click={() => onBuyMore(bet.assetId, qty)}>{$t('esports.buyMore')}</button>
            </div>
          </div>
        {/each}
      </div>
    </section>
  {/if}

  <section class="panel esports-head">
    <div class="panel-head"><div><p class="eyebrow">{$t('esports.liveFrom')}</p><h2>{$t('esports.title')}<InfoTip placement="bottom" text={$t('esports.marketTip')} /></h2></div><Trophy size={18} /></div>
    <p class="esports-sub">{$t('esports.sub', { amount: formatMoney(100) })}</p>
    <div class="filter-bar">
      <div class="league-filter">
        {#each leagueOptions as league}
          <button class:active={selectedLeagues.includes(league)} type="button" title={$t('esports.leagueFilterTitle', { league })} on:click={() => onToggleLeague(league)}>{league}</button>
        {/each}
      </div>
      <div class="filter-toggles">
        <button class="fav-toggle" class:active={showAllLeagues} type="button" title={$t('esports.allLeaguesTitle')} on:click={() => (showAllLeagues = !showAllLeagues)}>
          {$t('esports.allLeagues')}
        </button>
        <button class="fav-toggle" class:active={showOnlyFavorites} type="button" title={$t('esports.onlyFavoritesTitle')} on:click={() => (showOnlyFavorites = !showOnlyFavorites)}>
          <Star size={14} fill={showOnlyFavorites ? 'currentColor' : 'none'} /> {$t('esports.onlyFavorites')}
        </button>
      </div>
    </div>
  </section>

  {#if loading}
    <div class="match-grid">
      {#each Array(6) as _}<div class="match-card skeleton"></div>{/each}
    </div>
  {:else if error}
    <p class="empty-state">{error}</p>
  {:else if filteredMatches.length === 0}
    <p class="empty-state">{$t('esports.noMatches')}</p>
  {:else}
    <div class="match-grid">
      {#each filteredMatches as match (match.id)}
        {@const confirmTeam = pendingTeam(match)}
        {@const bettingClosed = match.team1.priceCents >= 100 || match.team2.priceCents >= 100}
        {@const displayProbability = displayProbabilities(match)}
        <article class="match-card" class:live={match.state === 'inProgress'} class:pending={!!confirmTeam}>
          <header class="match-top">
            <span class="league">{match.league}{match.bestOf ? ` · BO${match.bestOf}` : ''}</span>
            <span class="when">
              {#if match.state === 'inProgress'}<Radio size={12} />{/if}
              {timeLabel(match.startTime, match.state, $locale)}
            </span>
            <button
              class="details-toggle"
              class:active={expandedDetailsId === match.id}
              type="button"
              aria-expanded={expandedDetailsId === match.id}
              aria-controls={`match-details-${match.id}`}
              title={$t('esports.detailsTitle')}
              on:click={() => toggleDetails(match.id)}
            >
              <ChevronDown size={14} /> {$t('esports.details')}
            </button>
          </header>

          <div class="vs" class:no-odds={!match.hasOdds}>
            {#each [match.team1, match.team2] as team, i}
              <div class="team-side" class:right={i === 1}>
                <button class="roundel"
                        type="button"
                        disabled={!match.hasOdds || team.priceCents <= 0 || bettingClosed || refreshingId === match.id}
                         title={bettingClosed ? $t('esports.marketClosedTitle') : match.hasOdds && team.priceCents > 0 ? $t('esports.betOnTitle', { code: team.code, pct: i === 0 ? displayProbability.team1 : displayProbability.team2 }) : $t('esports.noQuoteTitle')}
                        on:click={() => startBet(match, team)}>
                  {#if team.image}<img src={team.image} alt="" width="58" height="58" loading="lazy" />{:else}{team.code.slice(0, 3)}{/if}
                </button>
                <div class="team-name">
                  <button class="star" class:on={favoriteTeams.includes(team.code)} type="button" title={$t('esports.markFavTitle', { code: team.code })} on:click={() => onToggleFavorite(team.code)}>
                    <Star size={12} fill={favoriteTeams.includes(team.code) ? 'currentColor' : 'none'} />
                  </button>
                  <strong>{team.code}</strong>
                </div>
                <small>{team.name}</small>
              </div>
              {#if i === 0}<span class="vs-badge">VS</span>{/if}
            {/each}
          </div>

          {#if match.hasOdds && (match.team1.priceCents > 0 || match.team2.priceCents > 0)}
            <div class="prob-wrap" title={$t('esports.probWrapTitle')}>
              <svg class="prob-bar" viewBox="0 0 100 14" role="img" aria-label={$t('esports.probAria', { a: `${displayProbability.team1}%`, b: `${displayProbability.team2}%` })}>
                <rect class="seg a" x="0" y="0" width={displayProbability.team1} height="14" />
                <rect class="seg b" x={displayProbability.team1} y="0" width={displayProbability.team2} height="14" />
              </svg>
              <div class="prob-legend">
                <span class="a">{displayProbability.team1}% · {formatMoney(match.team1.priceCents)}</span>
                <span class="src">{$t('esports.polymarketSrc')}</span>
                <span class="b">{displayProbability.team2}% · {formatMoney(match.team2.priceCents)}</span>
              </div>
            </div>
          {:else}
            <div class="no-odds-box">{$t('esports.noOddsBox')}</div>
          {/if}

          {#if expandedDetailsId === match.id}
            <section class="match-details" id={`match-details-${match.id}`} aria-live="polite">
              {#if detailsLoadingId === match.id}
                <span class="refreshing">{$t('esports.loadingDetails')}</span>
              {:else if detailsErrors[match.id]}
                <span class="details-error">{detailsErrors[match.id]}</span>
              {:else if matchDetails[match.id]}
                {@const details = matchDetails[match.id]}
                <div class="details-score">
                  <span>{details.team1Code || match.team1.code}</span>
                  <strong>{details.team1Score} : {details.team2Score}</strong>
                  <span>{details.team2Code || match.team2.code}</span>
                </div>
                {#if details.games.length > 0}
                  <div class="game-list">
                    {#each details.games as game}
                      <div class="game-row">
                        <span>{$t('esports.gameNumber', { number: game.gameNumber })}</span>
                        <span>{game.state || $t('esports.unknownState')}</span>
                        <code>{game.gameId}</code>
                      </div>
                    {/each}
                  </div>
                {/if}
                {#if details.videos.length > 0}
                  <div class="video-links">
                    {#each details.videos as video}
                      <a href={video.url} target="_blank" rel="noopener noreferrer">
                        <ExternalLink size={13} /> {videoLabel(video)}
                      </a>
                    {/each}
                  </div>
                {:else}
                  <span class="details-muted">{$t('esports.noVideos')}</span>
                {/if}
              {/if}
            </section>
          {/if}

          {#if confirmTeam}
            <footer class="confirm-bar">
              {#if refreshingId === match.id}
                <span class="refreshing"><RefreshCw size={13} /> {$t('esports.fetchingQuote')}</span>
              {:else if bettingClosed}
                <span class="refreshing">{$t('esports.marketClosed')}</span>
                <button class="ghost" type="button" title={$t('esports.closeWindowTitle')} on:click={cancelBet}>{$t('common.close')}</button>
              {:else if confirmTeam.priceCents > 0}
                <div class="confirm-info">
                  <div class="confirm-heading">
                    <strong>{confirmTeam.code} · {confirmTeam.code === match.team1.code ? displayProbability.team1 : displayProbability.team2}%</strong>
                    <small>{$t('esports.simulatedPrice')}</small>
                  </div>
                  <label class="contract-control" title={$t('esports.contractsFieldTitle')}>
                    <span>{$t('esports.contracts')}</span>
                    <input type="number" min="1" step="1" value={stakeFor(match.id)} title={$t('esports.contractsFieldTitle')} on:input={(e) => (stakes[match.id] = Math.max(1, Math.floor(Number(e.currentTarget.value)) || 1))} />
                  </label>
                  <dl class="order-breakdown">
                    <div><dt>{$t('esports.pricePerContract')}</dt><dd>{formatMoney(confirmTeam.priceCents)}</dd></div>
                    <div><dt>{$t('esports.subtotal')}</dt><dd>{stakeFor(match.id)} × {formatMoney(confirmTeam.priceCents)} = {formatMoney(grossCents(confirmTeam.priceCents, stakeFor(match.id)))}</dd></div>
                    <div><dt>{$t('esports.fee')}</dt><dd>{formatMoney(feeCents(confirmTeam.priceCents, stakeFor(match.id)))}</dd></div>
                    <div class="total"><dt>{$t('esports.total')}</dt><dd>{formatMoney(costCents(confirmTeam.priceCents, stakeFor(match.id)))}</dd></div>
                    <div><dt>{$t('esports.maxPayout')}</dt><dd>{formatMoney(stakeFor(match.id) * 100)}</dd></div>
                    <div><dt>{$t('esports.possibleProfit')}</dt><dd>{formatMoney((stakeFor(match.id) * 100) - costCents(confirmTeam.priceCents, stakeFor(match.id)))}</dd></div>
                  </dl>
                </div>
                <div class="confirm-actions">
                  <button class="ghost" type="button" title={$t('esports.cancelBetTitle')} on:click={cancelBet}>{$t('common.cancel')}</button>
                  <button class="confirm" type="button" title={$t('esports.confirmTitle')} disabled={!canAfford(confirmTeam.priceCents, stakeFor(match.id))} on:click={() => confirmBet(match)}>{$t('esports.confirm')}</button>
                </div>
              {:else}
                <span class="refreshing">{$t('esports.noQuoteAvailable')}</span>
                <button class="ghost" type="button" title={$t('esports.closeWindowTitle')} on:click={cancelBet}>{$t('common.close')}</button>
              {/if}
            </footer>
          {:else if match.hasOdds && !bettingClosed}
            <footer class="match-foot">
              <span class="hint">{$t('esports.selectTeamHint')}</span>
            </footer>
          {:else if bettingClosed}
            <footer class="match-foot">
              <span class="hint">{$t('esports.marketClosed')}</span>
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

  .filter-toggles {
    display: flex;
    gap: 0.4rem;
    flex-wrap: wrap;
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
    padding: 1rem 1.1rem;
    border: 1px solid hsla(var(--green-hsl), 0.22);
    border-radius: var(--r);
    background:
      radial-gradient(120px 90px at 88% -10%, hsla(var(--green-hsl), 0.12), transparent 70%),
      linear-gradient(180deg, rgba(255, 255, 255, 0.018), transparent),
      var(--panel);
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

  .details-toggle {
    display: inline-flex;
    align-items: center;
    gap: 0.2rem;
    min-height: 1.8rem;
    padding: 0 0.45rem;
    border: 1px solid var(--line);
    border-radius: 6px;
    color: var(--muted);
    background: var(--bg-2);
    white-space: nowrap;
  }

  .details-toggle :global(svg) {
    transition: transform 120ms ease;
  }

  .details-toggle.active {
    color: var(--green);
    border-color: var(--green-soft);
  }

  .details-toggle.active :global(svg) {
    transform: rotate(180deg);
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

  /* VS layout: roundel — VS badge — roundel */
  .vs {
    display: grid;
    grid-template-columns: 1fr auto 1fr;
    align-items: start;
    gap: 0.5rem;
  }

  .team-side {
    display: grid;
    justify-items: center;
    gap: 0.3rem;
    min-width: 0;
  }

  .roundel {
    display: grid;
    place-items: center;
    width: 58px;
    height: 58px;
    padding: 0;
    border: 1px solid var(--line-2);
    border-radius: 50%;
    overflow: hidden;
    font-family: var(--font-display);
    font-weight: 800;
    font-size: 1.1rem;
    letter-spacing: 0.01em;
    color: #fff;
    background: #080d18;
    box-shadow: inset 0 1px 0 0 hsla(0, 0%, 100%, 0.06);
    transition: 140ms ease;
  }

  .roundel:hover:not(:disabled) {
    transform: translateY(-2px);
    box-shadow: 0 6px 18px -6px rgba(0, 0, 0, 0.6), inset 0 1px 0 0 hsla(0, 0%, 100%, 0.15);
  }

  .roundel:disabled {
    cursor: default;
  }

  .roundel img {
    width: 100%;
    height: 100%;
    padding: 0.35rem;
    object-fit: contain;
    background: #080d18;
  }

  .vs.no-odds .roundel {
    filter: grayscale(1);
    opacity: 0.75;
  }

  .team-name {
    display: inline-flex;
    align-items: center;
    gap: 0.2rem;
  }

  .team-name strong {
    font-size: 0.92rem;
  }

  .star {
    display: grid;
    place-items: center;
    width: 1.2rem;
    height: 1.2rem;
    border: 0;
    border-radius: 5px;
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

  .team-side small {
    color: var(--muted);
    font-size: 0.68rem;
    text-align: center;
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .vs-badge {
    align-self: center;
    display: grid;
    place-items: center;
    width: 36px;
    height: 36px;
    border-radius: 50%;
    border: 1px solid var(--line);
    color: var(--muted);
    font-family: var(--font-display);
    font-weight: 700;
    font-size: 0.72rem;
    background: var(--bg-2);
  }

  /* two-colour win-probability bar */
  .prob-wrap {
    display: grid;
    gap: 0.3rem;
  }

  .prob-bar {
    display: block;
    width: 100%;
    height: 14px;
    border-radius: 999px;
    overflow: hidden;
    background: var(--bg-2);
  }

  .prob-bar .seg {
    stroke: none;
  }

  .prob-bar .seg.a {
    fill: #39db8f;
  }

  .prob-bar .seg.b {
    fill: #fb567f;
  }

  .prob-legend {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    font-size: 0.74rem;
  }

  .prob-legend .a {
    color: var(--green);
    font-weight: 600;
  }

  .prob-legend .b {
    color: var(--red);
    font-weight: 600;
  }

  .prob-legend .src {
    color: var(--tertiary, #64748b);
    font-size: 0.66rem;
  }

  .no-odds-box {
    padding: 0.6rem 0.75rem;
    border: 1px dashed var(--line-2);
    border-radius: var(--r-sm);
    color: var(--muted);
    font-size: 0.76rem;
    text-align: center;
  }

  .match-details {
    display: grid;
    gap: 0.55rem;
    padding: 0.65rem 0.7rem;
    border: 1px solid var(--line);
    border-radius: var(--r-sm);
    background: var(--bg-2);
  }

  .details-score {
    display: grid;
    grid-template-columns: 1fr auto 1fr;
    align-items: center;
    gap: 0.5rem;
    color: var(--muted);
    font-size: 0.78rem;
  }

  .details-score span:last-child {
    text-align: right;
  }

  .details-score strong {
    color: var(--text);
    font-size: 1rem;
  }

  .game-list {
    display: grid;
    gap: 0.25rem;
  }

  .game-row {
    display: grid;
    grid-template-columns: 4rem 1fr auto;
    gap: 0.45rem;
    align-items: center;
    color: var(--muted);
    font-size: 0.72rem;
  }

  .game-row code {
    max-width: 8rem;
    overflow: hidden;
    color: var(--tertiary, #64748b);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .video-links {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
  }

  .video-links a {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    min-height: 1.8rem;
    padding: 0 0.5rem;
    border: 1px solid var(--line);
    border-radius: 6px;
    color: var(--green);
    font-size: 0.72rem;
    text-decoration: none;
  }

  .video-links a:hover {
    border-color: var(--green-soft);
    background: var(--green-soft);
  }

  .details-error,
  .details-muted {
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

  .match-foot .hint {
    color: var(--muted);
    font-size: 0.74rem;
  }

  .confirm-bar {
    display: grid;
    gap: 0.75rem;
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
    gap: 0.6rem;
  }

  .confirm-heading {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 0.6rem;
  }

  .confirm-heading small {
    color: var(--muted);
    font-size: 0.72rem;
  }

  .contract-control {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    color: var(--muted);
    font-size: 0.76rem;
  }

  .contract-control input {
    width: 5rem;
    min-height: 2.1rem;
    padding: 0 0.5rem;
    border: 1px solid var(--line-2);
    border-radius: 6px;
    color: var(--text);
    background: var(--bg-2);
    outline: none;
  }

  .order-breakdown {
    display: grid;
    gap: 0.25rem;
    margin: 0;
  }

  .order-breakdown div {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 0.75rem;
    color: var(--muted);
    font-size: 0.72rem;
  }

  .order-breakdown dt,
  .order-breakdown dd {
    margin: 0;
  }

  .order-breakdown dd {
    color: var(--text);
    text-align: right;
  }

  .order-breakdown .total {
    margin-top: 0.15rem;
    padding-top: 0.35rem;
    border-top: 1px solid var(--line-2);
    font-weight: 700;
  }

  .confirm-actions {
    display: flex;
    justify-content: flex-end;
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
