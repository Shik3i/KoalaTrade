<script lang="ts">
  import {
    Activity,
    Bell,
    CandlestickChart,
    CloudUpload,
    Layers3,
    LineChart,
    RotateCcw,
    Search,
    ShieldCheck,
    Trophy,
    WalletCards
  } from '@lucide/svelte';
  import { onMount } from 'svelte';
  import { fetchMarkets, fetchPublicConfig, syncPortfolio, type Market, type PublicConfig } from './lib/api';
  import { loadClientId, loadPortfolio, resetPortfolio, savePortfolio } from './lib/portfolio-db';
  import {
    applyTrade,
    createInitialPortfolio,
    formatMoney,
    formatPercentFromBps,
    summarizePortfolio,
    type PortfolioSnapshot
  } from './lib/portfolio';

  const fallbackMarkets: Market[] = [
    { assetId: 'crypto:btc', symbol: 'BTC', name: 'Bitcoin', kind: 'crypto', source: 'local', priceCents: 6_142_020, changeBps: 280, updatedAt: new Date(0).toISOString() },
    { assetId: 'etf:spy', symbol: 'SPY', name: 'S&P 500 ETF', kind: 'etf', source: 'local', priceCents: 54_618, changeBps: 40, updatedAt: new Date(0).toISOString() },
    { assetId: 'commodity:gld', symbol: 'GLD', name: 'Gold Trust', kind: 'commodity', source: 'local', priceCents: 21_492, changeBps: -20, updatedAt: new Date(0).toISOString() },
    { assetId: 'event:pmkt', symbol: 'PMKT', name: 'Event Markets', kind: 'event', source: 'local', priceCents: 62, changeBps: 0, updatedAt: new Date(0).toISOString() }
  ];

  let config: PublicConfig | null = null;
  let configError = '';
  let markets: Market[] = fallbackMarkets;
  let marketsError = '';
  let portfolio: PortfolioSnapshot | null = null;
  let portfolioError = '';
  let selectedAssetId = fallbackMarkets[0].assetId;
  let orderSide: 'buy' | 'sell' = 'buy';
  let orderQuantity = 1;
  let orderError = '';
  let isSyncing = false;
  let syncError = '';
  let syncMessage = 'Sync is off until you choose it';

  onMount(async () => {
    try {
      config = await fetchPublicConfig();
    } catch (error) {
      configError = error instanceof Error ? error.message : 'Backend unavailable';
    }

    try {
      markets = await fetchMarkets();
      if (!markets.some((market) => market.assetId === selectedAssetId)) {
        selectedAssetId = markets[0]?.assetId ?? fallbackMarkets[0].assetId;
      }
    } catch (error) {
      marketsError = error instanceof Error ? error.message : 'Market data unavailable';
      markets = fallbackMarkets;
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
  $: syncLabel = portfolioError ? 'Fallback' : isSyncing ? 'Syncing' : summary.localTransactionCount > 0 ? 'Queued' : 'Synced';
  $: selectedMarket = markets.find((item) => item.assetId === selectedAssetId) ?? markets[0] ?? fallbackMarkets[0];
  $: normalizedOrderQuantity = Number.isFinite(Number(orderQuantity)) ? Number(orderQuantity) : 0;
  $: estimatedOrderValue = Math.round(normalizedOrderQuantity * selectedMarket.priceCents);
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
      state: summary.localTransactionCount > 0 ? 'Ready' : 'Clear'
    }
  ];
  $: activity = [
    portfolio ? `Local portfolio opened ${formatUpdatedAt(portfolio.updatedAt)}` : 'Opening local portfolio',
    portfolio?.transactions[0]
      ? `${portfolio.transactions[0].side.toUpperCase()} ${portfolio.transactions[0].quantity} ${portfolio.transactions[0].symbol} stored locally`
      : 'No simulated trades yet',
    syncMessage,
    marketsError ? 'Using local fallback market data' : `Market data source: ${config?.marketDataSource ?? selectedMarket.source}`,
    config ? 'Backend config endpoint connected' : 'Running with local defaults'
  ];

  async function handleResetPortfolio() {
    const startingCashCents = config?.startingCashCents ?? portfolio?.startingCashCents ?? 1_000_000;
    portfolio = await resetPortfolio(startingCashCents);
    portfolioError = '';
    orderError = '';
    syncError = '';
    syncMessage = 'Local portfolio reset';
  }

  async function handleSubmitOrder() {
    orderError = '';
    if (!portfolio) {
      orderError = 'Portfolio is still opening';
      return;
    }

    try {
      const nextPortfolio = applyTrade(portfolio, {
        id: crypto.randomUUID(),
        assetId: selectedMarket.assetId,
        symbol: selectedMarket.symbol,
        name: selectedMarket.name,
        kind: selectedMarket.kind,
        side: orderSide,
        quantity: normalizedOrderQuantity,
        priceCents: selectedMarket.priceCents
      });
      await savePortfolio(nextPortfolio);
      portfolio = nextPortfolio;
      syncError = '';
      syncMessage = 'Local trade queued for optional sync';
    } catch (error) {
      orderError = error instanceof Error ? error.message : 'Unable to place simulated order';
    }
  }

  async function handleSyncPortfolio() {
    syncError = '';
    if (!portfolio) {
      syncError = 'Portfolio is still opening';
      return;
    }

    isSyncing = true;
    try {
      const clientId = await loadClientId();
      const synced = await syncPortfolio(clientId, portfolio);
      await savePortfolio(synced);
      portfolio = synced;
      syncMessage = `Synced ${formatUpdatedAt(synced.updatedAt)}`;
    } catch (error) {
      syncError = error instanceof Error ? error.message : 'Portfolio sync failed';
      syncMessage = 'Sync failed; local state is unchanged';
    } finally {
      isSyncing = false;
    }
  }

  function formatUpdatedAt(value: string) {
    return new Intl.DateTimeFormat('en-US', {
      hour: '2-digit',
      minute: '2-digit'
    }).format(new Date(value));
  }

  function marketTone(changeBps: number) {
    if (changeBps > 0) return 'up';
    if (changeBps < 0) return 'down';
    return 'flat';
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
          {#each markets as item}
            <button
              class:selected={selectedMarket.assetId === item.assetId}
              class="market-row market-button"
              type="button"
              on:click={() => (selectedAssetId = item.assetId)}
            >
              <div>
                <strong>{item.symbol}</strong>
                <span>{item.name}</span>
              </div>
              <div class="market-price">
                <strong>{formatMoney(item.priceCents)}</strong>
                <span class={marketTone(item.changeBps)}>{formatPercentFromBps(item.changeBps)}</span>
              </div>
            </button>
          {/each}
        </div>
      </section>

      <section class="trade-panel panel" aria-label="Simulated order ticket">
        <div class="panel-heading">
          <div>
            <p class="eyebrow">Order</p>
            <h2>{selectedMarket.symbol}</h2>
          </div>
          <CandlestickChart size={19} />
        </div>

        <form class="order-form" on:submit|preventDefault={handleSubmitOrder}>
          <div class="segmented" aria-label="Order side">
            <button class:active={orderSide === 'buy'} type="button" on:click={() => (orderSide = 'buy')}>Buy</button>
            <button class:active={orderSide === 'sell'} type="button" on:click={() => (orderSide = 'sell')}>Sell</button>
          </div>

          <label class="field">
            <span>Quantity</span>
            <input bind:value={orderQuantity} min="0.0001" step="0.0001" type="number" />
          </label>

          <div class="order-estimate">
            <span>Estimated value</span>
            <strong>{formatMoney(estimatedOrderValue)}</strong>
          </div>

          {#if orderError}
            <p class="form-error">{orderError}</p>
          {/if}

          <button class="primary-button" type="submit">{orderSide === 'buy' ? 'Buy' : 'Sell'} {selectedMarket.symbol}</button>
        </form>
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
          <div class="panel-actions">
            <button class="icon-button" type="button" aria-label="Sync portfolio" title="Sync portfolio" disabled={isSyncing || !portfolio || !!configError} on:click={handleSyncPortfolio}>
              <CloudUpload size={18} />
            </button>
            <button class="icon-button" type="button" aria-label="Reset portfolio" title="Reset portfolio" on:click={handleResetPortfolio}>
              <RotateCcw size={18} />
            </button>
          </div>
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
        {#if syncError}
          <p class="form-error">{syncError}</p>
        {/if}
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
          {#if marketsError}
            <li class="warning">{marketsError}</li>
          {/if}
        </ul>
      </section>
    </section>
  </section>
</main>
