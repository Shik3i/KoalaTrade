<script lang="ts">
  import {
    Activity,
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
  let marketQuery = '';
  let orderSide: 'buy' | 'sell' = 'buy';
  let orderQuantity = 1;
  let orderError = '';
  let isSyncing = false;
  let syncError = '';
  let syncMessage = 'Sync off';
  let activeView: 'landing' | 'desk' = 'landing';

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

    try {
      portfolio = await loadPortfolio(config?.startingCashCents ?? 1_000_000);
    } catch (error) {
      portfolioError = error instanceof Error ? error.message : 'Local portfolio unavailable';
      portfolio = createInitialPortfolio(config?.startingCashCents ?? 1_000_000);
    }
  });

  $: summary = summarizePortfolio(portfolio ?? createInitialPortfolio(config?.startingCashCents ?? 1_000_000));
  $: selectedMarket = markets.find((item) => item.assetId === selectedAssetId) ?? markets[0] ?? fallbackMarkets[0];
  $: query = marketQuery.trim().toLowerCase();
  $: filteredMarkets = query
    ? markets.filter((market) => `${market.symbol} ${market.name} ${market.kind}`.toLowerCase().includes(query))
    : markets;
  $: normalizedOrderQuantity = Number.isFinite(Number(orderQuantity)) ? Number(orderQuantity) : 0;
  $: estimatedOrderValue = Math.round(normalizedOrderQuantity * selectedMarket.priceCents);
  $: syncLabel = portfolioError ? 'Fallback' : isSyncing ? 'Syncing' : summary.localTransactionCount > 0 ? 'Queued' : 'Synced';
  $: positionList = portfolio?.positions ?? [];
  $: transactionList = portfolio?.transactions.slice(0, 6) ?? [];
  $: exposureRows = [
    ['Cash', formatMoney(summary.cashCents), summary.totalEquityCents > 0 ? formatPercentFromBps(Math.round((summary.cashCents / summary.totalEquityCents) * 10_000)) : '0.00%'],
    ['Positions', formatMoney(summary.positionsValueCents), `${summary.openPositions} open`],
    ['Pending', `${summary.localTransactionCount} tx`, syncLabel]
  ];
  $: systemItems = [
    config ? `API online · ${config.marketDataSource}` : 'Local defaults',
    marketsError ? 'Market fallback active' : `${markets.length} markets loaded`,
    portfolio ? `Portfolio updated ${formatUpdatedAt(portfolio.updatedAt)}` : 'Opening portfolio',
    syncMessage
  ];

  async function handleResetPortfolio() {
    portfolio = await resetPortfolio(config?.startingCashCents ?? portfolio?.startingCashCents ?? 1_000_000);
    orderError = '';
    syncError = '';
    syncMessage = 'Portfolio reset';
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
      syncMessage = 'Trade queued';
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
      const synced = await syncPortfolio(await loadClientId(), portfolio);
      await savePortfolio(synced);
      portfolio = synced;
      syncMessage = `Synced ${formatUpdatedAt(synced.updatedAt)}`;
    } catch (error) {
      syncError = error instanceof Error ? error.message : 'Portfolio sync failed';
      syncMessage = 'Sync failed';
    } finally {
      isSyncing = false;
    }
  }

  function formatUpdatedAt(value: string) {
    return new Intl.DateTimeFormat('en-US', { hour: '2-digit', minute: '2-digit' }).format(new Date(value));
  }

  function marketTone(changeBps: number) {
    if (changeBps > 0) return 'up';
    if (changeBps < 0) return 'down';
    return 'flat';
  }
</script>

{#if activeView === 'landing'}
  <main class="landing-shell">
    <header class="landing-nav">
      <div class="brand">
        <img src="/icons/koalatrade.svg" alt="" width="38" height="38" />
        <div>
          <strong>KoalaTrade</strong>
          <span>Paper markets</span>
        </div>
      </div>
      <button class="nav-action" type="button" on:click={() => (activeView = 'desk')}>Trading Desk öffnen</button>
    </header>

    <section class="landing-hero" aria-label="KoalaTrade introduction">
      <div class="landing-copy">
        <p class="eyebrow">Virtual trading cockpit</p>
        <h1>Trainiere Märkte, ohne echtes Geld zu riskieren.</h1>
        <p>
          KoalaTrade verbindet Aktien, ETFs, Crypto, Rohstoffe und Event-Märkte in einem schnellen Paper-Trading-Desk.
        </p>
        <div class="landing-actions">
          <button class="primary-button" type="button" on:click={() => (activeView = 'desk')}>Desk starten</button>
          <span>{config ? 'Live API bereit' : 'Lädt lokale Session'}</span>
        </div>
      </div>

      <div class="landing-terminal" aria-label="Product preview">
        <div class="terminal-top">
          <span>{selectedMarket.symbol}</span>
          <strong>{formatMoney(selectedMarket.priceCents)}</strong>
          <em class={marketTone(selectedMarket.changeBps)}>{formatPercentFromBps(selectedMarket.changeBps)}</em>
        </div>
        <div class="preview-chart" aria-hidden="true"><span></span><span></span><span></span><span></span></div>
        <div class="preview-grid">
          <div><span>Equity</span><strong>{formatMoney(summary.totalEquityCents)}</strong></div>
          <div><span>Cash</span><strong>{formatMoney(summary.cashCents)}</strong></div>
          <div><span>Markets</span><strong>{markets.length}</strong></div>
        </div>
      </div>
    </section>

    <section class="landing-bands" aria-label="Core product areas">
      <article>
        <CandlestickChart size={20} />
        <strong>Multi-asset watchlist</strong>
        <span>Ein normalisiertes Marktmodell für Crypto, ETFs, Rohstoffe und Event-Märkte.</span>
      </article>
      <article>
        <WalletCards size={20} />
        <strong>Portfolio ledger</strong>
        <span>Orders, Cash, Positionen und Sync-Status getrennt statt in einem Demo-Klotz.</span>
      </article>
      <article>
        <Trophy size={20} />
        <strong>Leaderboard-ready</strong>
        <span>Das Datenmodell ist vorbereitet für spätere Seasons, Accounts und Rankings.</span>
      </article>
    </section>
  </main>
{:else}
  <main class="terminal-shell">
    <aside class="rail" aria-label="Primary">
      <div class="brand">
        <img src="/icons/koalatrade.svg" alt="" width="38" height="38" />
        <div>
          <strong>KoalaTrade</strong>
          <span>Trading Desk</span>
        </div>
      </div>

      <nav class="nav-list" aria-label="Trading sections">
        <button class="nav-item" type="button" on:click={() => (activeView = 'landing')}><LineChart size={18} /> Landing</button>
        <button class="nav-item active" type="button"><CandlestickChart size={18} /> Markets</button>
        <button class="nav-item" type="button"><WalletCards size={18} /> Portfolio</button>
        <button class="nav-item" type="button"><Trophy size={18} /> Boards</button>
      </nav>
    </aside>

    <section class="workspace" aria-label="Trading workspace">
      <header class="command-bar">
        <label class="search" aria-label="Search markets">
          <Search size={18} />
          <input bind:value={marketQuery} type="search" placeholder="Search BTC, ETF, gold, events" />
        </label>
        <div class="command-actions">
          <span class:online={config} class="status-pill">{config ? 'API online' : 'Local mode'}</span>
          <button class="icon-button" type="button" aria-label="Sync portfolio" title="Sync portfolio" disabled={isSyncing || !portfolio || !!configError} on:click={handleSyncPortfolio}>
            <CloudUpload size={18} />
          </button>
          <button class="icon-button" type="button" aria-label="Reset portfolio" title="Reset portfolio" on:click={handleResetPortfolio}>
            <RotateCcw size={18} />
          </button>
        </div>
      </header>

      <section class="metrics-strip" aria-label="Portfolio metrics">
        <div class="metric primary"><span>Equity</span><strong>{formatMoney(summary.totalEquityCents)}</strong></div>
        <div class="metric">
          <span>Return</span>
          <strong class:up={summary.totalReturnCents > 0} class:down={summary.totalReturnCents < 0}>{formatPercentFromBps(summary.totalReturnBps)}</strong>
        </div>
        <div class="metric"><span>Cash</span><strong>{formatMoney(summary.cashCents)}</strong></div>
        <div class="metric"><span>Sync</span><strong>{configError ? 'Offline' : syncLabel}</strong></div>
      </section>

      <section class="desk-grid">
        <section class="panel market-panel" aria-label="Markets">
          <div class="panel-head">
            <div><p class="eyebrow">Markets</p><h2>Watchlist</h2></div>
            <span>{filteredMarkets.length}</span>
          </div>
          <div class="market-list">
            {#each filteredMarkets as item}
              <button class:selected={selectedMarket.assetId === item.assetId} class="market-row" type="button" on:click={() => (selectedAssetId = item.assetId)}>
                <span class="symbol">{item.symbol}</span>
                <span>{item.name}</span>
                <strong>{formatMoney(item.priceCents)}</strong>
                <em class={marketTone(item.changeBps)}>{formatPercentFromBps(item.changeBps)}</em>
              </button>
            {/each}
          </div>
        </section>

        <section class="trade-stack">
          <section class="instrument panel" aria-label="Selected market">
            <div>
              <p class="eyebrow">{selectedMarket.kind} · {selectedMarket.source}</p>
              <h1>{selectedMarket.symbol}</h1>
              <span>{selectedMarket.name}</span>
            </div>
            <div class="instrument-price">
              <strong>{formatMoney(selectedMarket.priceCents)}</strong>
              <span class={marketTone(selectedMarket.changeBps)}>{formatPercentFromBps(selectedMarket.changeBps)}</span>
            </div>
          </section>

          <section class="chart-panel panel" aria-label="Price surface">
            <div class="panel-head">
              <div><p class="eyebrow">Signal</p><h2>Session shape</h2></div>
              <Layers3 size={19} />
            </div>
            <div class="chart-surface" aria-hidden="true"><span></span><span></span><span></span><span></span></div>
          </section>

          <section class="order-panel panel" aria-label="Order ticket">
            <div class="panel-head">
              <div><p class="eyebrow">Order ticket</p><h2>{orderSide === 'buy' ? 'Buy' : 'Sell'} {selectedMarket.symbol}</h2></div>
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
              <div class="estimate"><span>Estimated value</span><strong>{formatMoney(estimatedOrderValue)}</strong></div>
              {#if orderError}<p class="form-error">{orderError}</p>{/if}
              <button class="primary-button" type="submit">{orderSide === 'buy' ? 'Buy' : 'Sell'} {selectedMarket.symbol}</button>
            </form>
          </section>
        </section>

        <section class="side-stack">
          <section class="panel" aria-label="Portfolio exposure">
            <div class="panel-head">
              <div><p class="eyebrow">Exposure</p><h2>Portfolio</h2></div>
              <WalletCards size={19} />
            </div>
            <div class="exposure-list">
              {#each exposureRows as row}
                <div class="exposure-row"><span>{row[0]}</span><strong>{row[1]}</strong><em>{row[2]}</em></div>
              {/each}
            </div>
            <div class="holding-list">
              {#if positionList.length === 0}
                <p class="empty-state">No open positions. Place a simulated order to start the book.</p>
              {:else}
                {#each positionList as position}
                  <div class="holding-row"><strong>{position.symbol}</strong><span>{position.quantity} · {formatMoney(position.lastPriceCents)}</span><em>{position.kind}</em></div>
                {/each}
              {/if}
            </div>
          </section>

          <section class="panel" aria-label="Recent transactions">
            <div class="panel-head">
              <div><p class="eyebrow">Ledger</p><h2>Recent orders</h2></div>
              <Activity size={19} />
            </div>
            <div class="ledger-list">
              {#if transactionList.length === 0}
                <p class="empty-state">No simulated trades yet.</p>
              {:else}
                {#each transactionList as transaction}
                  <div class="ledger-row"><strong>{transaction.side.toUpperCase()} {transaction.symbol}</strong><span>{transaction.quantity} @ {formatMoney(transaction.priceCents)}</span><em>{transaction.status}</em></div>
                {/each}
              {/if}
            </div>
          </section>

          <section class="panel system-panel" aria-label="System status">
            <div class="panel-head">
              <div><p class="eyebrow">System</p><h2>Status</h2></div>
              <ShieldCheck size={19} />
            </div>
            <ul>
              {#each systemItems as item}<li>{item}</li>{/each}
              {#if configError}<li class="warning">{configError}</li>{/if}
              {#if portfolioError}<li class="warning">{portfolioError}</li>{/if}
              {#if marketsError}<li class="warning">{marketsError}</li>{/if}
              {#if syncError}<li class="warning">{syncError}</li>{/if}
            </ul>
          </section>
        </section>
      </section>
    </section>
  </main>
{/if}
