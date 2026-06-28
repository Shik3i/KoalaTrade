<script lang="ts">
  import {
    Activity,
    Bell,
    CandlestickChart,
    Layers3,
    LineChart,
    RotateCcw,
    Search,
    ShieldCheck,
    Trophy,
    WalletCards
  } from '@lucide/svelte';
  import { onMount } from 'svelte';
  import { fetchPublicConfig, type PublicConfig } from './lib/api';
  import { loadPortfolio, resetPortfolio } from './lib/portfolio-db';
  import {
    createInitialPortfolio,
    formatMoney,
    formatPercentFromBps,
    summarizePortfolio,
    type PortfolioSnapshot
  } from './lib/portfolio';

  const watchlist = [
    { symbol: 'BTC', name: 'Bitcoin', price: '$61,420.20', change: '+2.8%', tone: 'up' },
    { symbol: 'SPY', name: 'S&P 500 ETF', price: '$546.18', change: '+0.4%', tone: 'up' },
    { symbol: 'GLD', name: 'Gold Trust', price: '$214.92', change: '-0.2%', tone: 'down' },
    { symbol: 'PMKT', name: 'Event Markets', price: '142 live', change: 'CLOB', tone: 'flat' }
  ];

  let config: PublicConfig | null = null;
  let configError = '';
  let portfolio: PortfolioSnapshot | null = null;
  let portfolioError = '';

  onMount(async () => {
    try {
      config = await fetchPublicConfig();
    } catch (error) {
      configError = error instanceof Error ? error.message : 'Backend unavailable';
    }

    const startingCashCents = config?.startingCashCents ?? 1_000_000;
    try {
      portfolio = await loadPortfolio(startingCashCents);
    } catch (error) {
      portfolioError = error instanceof Error ? error.message : 'Local portfolio unavailable';
      portfolio = createInitialPortfolio(startingCashCents);
    }
  });

  $: summary = summarizePortfolio(portfolio ?? createInitialPortfolio(config?.startingCashCents ?? 1_000_000));
  $: equityLabel = formatMoney(summary.totalEquityCents);
  $: returnLabel = formatPercentFromBps(summary.totalReturnBps);
  $: syncLabel = portfolioError ? 'Fallback' : summary.localTransactionCount > 0 ? 'Queued' : 'Local';
  $: positionRows = [
    {
      market: 'Cash',
      exposure: formatMoney(summary.cashCents),
      allocation: summary.totalEquityCents > 0 ? formatPercentFromBps(Math.round((summary.cashCents / summary.totalEquityCents) * 10_000)) : '0.00%',
      state: 'Available'
    },
    {
      market: 'Positions',
      exposure: formatMoney(summary.positionsValueCents),
      allocation: `${summary.openPositions} open`,
      state: summary.openPositions > 0 ? 'Active' : 'None yet'
    },
    {
      market: 'Pending sync',
      exposure: `${summary.localTransactionCount} local`,
      allocation: 'Opt-in',
      state: 'Private'
    }
  ];
  $: activity = [
    portfolio ? `Local portfolio opened ${formatUpdatedAt(portfolio.updatedAt)}` : 'Opening local portfolio',
    config ? 'Backend config endpoint connected' : 'Running with local defaults',
    'No external market data loaded yet'
  ];

  async function handleResetPortfolio() {
    const startingCashCents = config?.startingCashCents ?? portfolio?.startingCashCents ?? 1_000_000;
    portfolio = await resetPortfolio(startingCashCents);
    portfolioError = '';
  }

  function formatUpdatedAt(value: string) {
    return new Intl.DateTimeFormat('en-US', {
      hour: '2-digit',
      minute: '2-digit'
    }).format(new Date(value));
  }
</script>

<main class="app-shell">
  <aside class="sidebar" aria-label="Primary">
    <div class="brand">
      <img src="/icons/koalatrade.svg" alt="" width="38" height="38" />
      <div>
        <strong>KoalaTrade</strong>
        <span>Paper markets</span>
      </div>
    </div>

    <nav class="nav-list" aria-label="Trading sections">
      <a class="nav-item active" href="/">
        <LineChart size={18} />
        Dashboard
      </a>
      <a class="nav-item" href="/">
        <CandlestickChart size={18} />
        Markets
      </a>
      <a class="nav-item" href="/">
        <WalletCards size={18} />
        Portfolio
      </a>
      <a class="nav-item" href="/">
        <Trophy size={18} />
        Leaderboard
      </a>
    </nav>

    <section class="privacy-panel" aria-label="Privacy status">
      <ShieldCheck size={20} />
      <div>
        <strong>Local-first</strong>
        <span>No CDN assets. Sync stays optional.</span>
      </div>
    </section>
  </aside>

  <section class="workspace">
    <header class="topbar">
      <label class="search" aria-label="Search markets">
        <Search size={18} />
        <input type="search" placeholder="Search stocks, crypto, events" />
      </label>
      <div class="topbar-actions">
        <span class:online={config} class="status-pill">{config ? 'API online' : 'Local mode'}</span>
        <button class="icon-button" type="button" aria-label="Notifications">
          <Bell size={18} />
        </button>
      </div>
    </header>

    <section class="hero-strip" aria-label="Portfolio overview">
      <div>
        <p class="eyebrow">Virtual equity</p>
        <h1>{equityLabel}</h1>
        <span class="muted">Local vault ready for simulated trades</span>
      </div>
      <div class="hero-metrics" aria-label="Portfolio metrics">
        <div>
          <span>Total</span>
          <strong class:up={summary.totalReturnCents > 0} class:down={summary.totalReturnCents < 0}>
            {returnLabel}
          </strong>
        </div>
        <div>
          <span>Cash</span>
          <strong>{formatMoney(summary.cashCents)}</strong>
        </div>
        <div>
          <span>Sync</span>
          <strong>{configError ? 'Offline' : syncLabel}</strong>
        </div>
      </div>
    </section>

    <section class="dashboard-grid">
      <section class="market-panel panel" aria-label="Watchlist">
        <div class="panel-heading">
          <div>
            <p class="eyebrow">Watchlist</p>
            <h2>Market pulse</h2>
          </div>
          <Activity size={19} />
        </div>

        <div class="market-list">
          {#each watchlist as item}
            <article class="market-row">
              <div>
                <strong>{item.symbol}</strong>
                <span>{item.name}</span>
              </div>
              <div class="market-price">
                <strong>{item.price}</strong>
                <span class={item.tone}>{item.change}</span>
              </div>
            </article>
          {/each}
        </div>
      </section>

      <section class="chart-panel panel" aria-label="Equity curve">
        <div class="panel-heading">
          <div>
            <p class="eyebrow">Equity curve</p>
            <h2>No trades yet</h2>
          </div>
          <Layers3 size={19} />
        </div>
        <div class="chart-surface" aria-hidden="true">
          <div class="chart-line"></div>
        </div>
      </section>

      <section class="positions-panel panel" aria-label="Portfolio positions">
        <div class="panel-heading">
          <div>
            <p class="eyebrow">Portfolio</p>
            <h2>Local state</h2>
          </div>
          <button class="icon-button" type="button" aria-label="Reset portfolio" title="Reset portfolio" on:click={handleResetPortfolio}>
            <RotateCcw size={18} />
          </button>
        </div>

        <div class="position-table">
          {#each positionRows as position}
            <div class="position-row">
              <span>{position.market}</span>
              <strong>{position.exposure}</strong>
              <span>{position.allocation}</span>
              <em>{position.state}</em>
            </div>
          {/each}
        </div>
      </section>

      <section class="activity-panel panel" aria-label="Activity">
        <div class="panel-heading">
          <div>
            <p class="eyebrow">System</p>
            <h2>Readiness</h2>
          </div>
          <ShieldCheck size={19} />
        </div>

        <ul>
          {#each activity as item}
            <li>{item}</li>
          {/each}
          {#if configError}
            <li class="warning">{configError}</li>
          {/if}
          {#if portfolioError}
            <li class="warning">{portfolioError}</li>
          {/if}
        </ul>
      </section>
    </section>
  </section>
</main>
