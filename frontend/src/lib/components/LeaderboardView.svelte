<script lang="ts">
  import { Award, RefreshCw, UserCircle2 } from '@lucide/svelte';
  import type { LeaderboardEntry, SessionUser } from '../api';
  import { formatMoney, formatPercentFromBps } from '../portfolio';
  import { t } from '../i18n';

  export let entries: LeaderboardEntry[] = [];
  export let loading = false;
  export let error = '';
  export let user: SessionUser | null = null;
  export let onRefresh: () => void;
  export let onGoToProfile: () => void;

  function tone(bps: number) {
    return bps > 0 ? 'up' : bps < 0 ? 'down' : 'flat';
  }

  function medal(rank: number) {
    if (rank === 1) return '<svg viewBox="0 0 20 20" width="16" height="16" fill="none"><circle cx="10" cy="10" r="9" fill="#fbbf24" stroke="#d97706" stroke-width="1.5"/><text x="10" y="14" text-anchor="middle" font-size="11" font-weight="700" fill="#92400e">1</text></svg>';
    if (rank === 2) return '<svg viewBox="0 0 20 20" width="16" height="16" fill="none"><circle cx="10" cy="10" r="9" fill="#94a3b8" stroke="#64748b" stroke-width="1.5"/><text x="10" y="14" text-anchor="middle" font-size="11" font-weight="700" fill="#fff">2</text></svg>';
    if (rank === 3) return '<svg viewBox="0 0 20 20" width="16" height="16" fill="none"><circle cx="10" cy="10" r="9" fill="#d97706" stroke="#b45309" stroke-width="1.5"/><text x="10" y="14" text-anchor="middle" font-size="11" font-weight="700" fill="#fff">3</text></svg>';
    return '';
  }
</script>

<div class="leaderboard">
  <section class="panel head">
    <div class="panel-head">
      <div><p class="eyebrow">{$t('leaderboard.competition')}</p><h2>{$t('leaderboard.title')}</h2></div>
      <button class="ghost-btn" type="button" title={$t('leaderboard.refreshTitle')} disabled={loading} on:click={onRefresh}>
        <RefreshCw size={15} /> {loading ? $t('leaderboard.loadingShort') : $t('common.refresh')}
      </button>
    </div>
    <p class="sub">{$t('leaderboard.subBefore')}<strong>{$t('leaderboard.subStrong')}</strong>{$t('leaderboard.subAfter')}</p>
    {#if !user}
      <button class="cta" type="button" on:click={onGoToProfile}>
        <UserCircle2 size={16} /> {$t('leaderboard.cta')}
      </button>
    {/if}
  </section>

  <section class="panel">
    {#if loading && entries.length === 0}
      <div class="rows">
        {#each Array(6) as _}<div class="skeleton-row"></div>{/each}
      </div>
    {:else if error}
      <p class="empty-state">{error}</p>
    {:else if entries.length === 0}
      <p class="empty-state">{$t('leaderboard.empty')}</p>
    {:else}
      <div class="table-head"><span>#</span><span>{$t('leaderboard.colTrader')}</span><span>{$t('leaderboard.colReturn')}</span><span>{$t('leaderboard.colEquity')}</span></div>
      <div class="rows">
        {#each entries as entry (entry.rank + entry.displayName)}
          <div class="row" class:you={entry.isYou}>
            <span class="rank">{@html medal(entry.rank)}{entry.rank}</span>
            <span class="name"><Award size={13} class={entry.rank <= 3 ? 'top' : 'dim'} />{entry.displayName}{#if entry.isYou}<em>{$t('leaderboard.you')}</em>{/if}</span>
            <strong class={tone(entry.totalReturnBps)}>{entry.totalReturnBps > 0 ? '+' : ''}{formatPercentFromBps(entry.totalReturnBps)}</strong>
            <span class="equity">{formatMoney(entry.totalEquityCents)}</span>
          </div>
        {/each}
      </div>
    {/if}
  </section>
</div>

<style>
  .leaderboard {
    display: grid;
    gap: 0.75rem;
    align-content: start;
    max-width: 44rem;
    margin: 0 auto;
    width: 100%;
  }

  .sub {
    margin: 0.25rem 0 0;
    color: var(--muted);
    font-size: 0.86rem;
    line-height: 1.5;
  }

  .cta {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    margin-top: 0.75rem;
    min-height: 2.2rem;
    padding: 0 0.9rem;
    border: 1px solid hsla(var(--green-hsl), 0.4);
    border-radius: 8px;
    color: var(--green);
    font-size: 0.85rem;
    background: var(--green-soft);
    transition: 120ms ease;
  }

  .cta:hover {
    background: hsla(var(--green-hsl), 0.16);
  }

  .ghost-btn {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    min-height: 2.1rem;
    padding: 0 0.75rem;
    border: 1px solid var(--line-2);
    border-radius: 6px;
    color: var(--text);
    font-size: 0.82rem;
    background: var(--panel-3);
  }

  .table-head,
  .row {
    display: grid;
    grid-template-columns: 3.2rem 1fr auto auto;
    gap: 0.75rem;
    align-items: center;
  }

  .table-head {
    padding: 0 0.7rem 0.5rem;
    color: var(--muted);
    font-size: 0.72rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .table-head span:nth-child(3),
  .table-head span:nth-child(4),
  .row strong,
  .row .equity {
    text-align: right;
  }

  .rows {
    display: grid;
    gap: 0.3rem;
  }

  .row {
    padding: 0.6rem 0.7rem;
    border: 1px solid var(--line);
    border-radius: var(--r-sm);
    background: var(--bg-2);
  }

  .row.you {
    border-color: hsla(var(--green-hsl), 0.4);
    background: linear-gradient(90deg, var(--green-soft), transparent 90%), var(--bg-2);
  }

  .rank {
    font-family: var(--font-display);
    font-weight: 700;
    font-size: 0.95rem;
    color: var(--soft);
  }

  .name {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-weight: 550;
  }

  .name :global(svg.top) {
    color: var(--amber);
  }

  .name :global(svg.dim) {
    color: var(--muted);
    opacity: 0.5;
  }

  .name em {
    padding: 0.05rem 0.4rem;
    border-radius: 999px;
    color: var(--green);
    font-style: normal;
    font-size: 0.66rem;
    background: var(--green-soft);
  }

  .row .equity {
    color: var(--soft);
    font-size: 0.88rem;
  }

  .skeleton-row {
    height: 2.6rem;
    border-radius: var(--r-sm);
    background: linear-gradient(100deg, rgba(255, 255, 255, 0.03) 30%, rgba(255, 255, 255, 0.07) 50%, rgba(255, 255, 255, 0.03) 70%);
    background-size: 200% 100%;
    animation: shimmer 1.3s infinite;
  }

  @media (max-width: 560px) {
    .table-head,
    .row {
      grid-template-columns: 2.4rem 1fr auto;
    }
    .table-head span:nth-child(4),
    .row .equity {
      display: none;
    }
  }
</style>
