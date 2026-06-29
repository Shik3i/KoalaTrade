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
  import { onDestroy, onMount } from 'svelte';
  import { fetchMarkets, fetchPublicConfig, fetchQuotes, fetchSyncedPortfolio, syncPortfolio, type Market, type PublicConfig } from './lib/api';
  import { loadClientId, loadPortfolio, resetPortfolio, savePortfolio } from './lib/portfolio-db';
  import {
    applyTrade,
    createInitialPortfolio,
    formatMoney,
    formatPercentFromBps,
    markPositionsToMarket,
    summarizePortfolio,
    PORTFOLIO_ID,
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
  let clientId = '';
  let quoteTimer: ReturnType<typeof setInterval> | undefined;

  onMount(async () => {
    try {
      clientId = await loadClientId();
    } catch (error) {
      syncError = error instanceof Error ? error.message : 'Sync client unavailable';
      syncMessage = 'Sync unavailable';
    }

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

    await restoreSyncedPortfolio();
    await refreshQuotes();
    quoteTimer = setInterval(refreshQuotes, 30_000);
  });

  onDestroy(() => {
    if (quoteTimer) clearInterval(quoteTimer);
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
  $: bookRows = [-38, -22, -9, 12, 26, 41].map((offset, index) => ({
    side: offset < 0 ? 'ask' : 'bid',
    priceCents: Math.max(1, selectedMarket.priceCents + Math.round((selectedMarket.priceCents * offset) / 10_000)),
    size: (0.18 + index * 0.11).toFixed(2),
    depth: `${36 + index * 9}%`
  }));
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

    await syncCurrentPortfolio(portfolio);
  }

  async function syncCurrentPortfolio(snapshot: PortfolioSnapshot) {
    isSyncing = true;
    try {
      const synced = await syncPortfolio(clientId || (await loadClientId()), snapshot);
      await savePortfolio(synced, { touchUpdatedAt: false });
      portfolio = synced;
      syncMessage = `Synced ${formatUpdatedAt(synced.updatedAt)}`;
    } catch (error) {
      syncError = error instanceof Error ? error.message : 'Portfolio sync failed';
      syncMessage = 'Sync failed';
    } finally {
      isSyncing = false;
    }
  }

  async function restoreSyncedPortfolio() {
    if (!portfolio || !clientId || configError) return;

    try {
      const synced = await fetchSyncedPortfolio(clientId, PORTFOLIO_ID);
      if (!synced) {
        syncMessage = 'Sync ready';
        return;
      }
      if (new Date(synced.updatedAt).getTime() > new Date(portfolio.updatedAt).getTime()) {
        await savePortfolio(synced, { touchUpdatedAt: false });
        portfolio = synced;
        syncMessage = `Restored ${formatUpdatedAt(synced.updatedAt)}`;
        return;
      }
      syncMessage = 'Local portfolio current';
    } catch (error) {
      syncError = error instanceof Error ? error.message : 'Portfolio restore failed';
      syncMessage = 'Sync unavailable';
    }
  }

  async function refreshQuotes() {
    if (markets.length === 0) return;

    try {
      const quotes = await fetchQuotes(markets.map((market) => market.assetId));
      const byAsset = new Map(quotes.map((quote) => [quote.assetId, quote]));
      markets = markets.map((market) => {
        const quote = byAsset.get(market.assetId);
        return quote
          ? { ...market, priceCents: quote.priceCents, changeBps: quote.changeBps, source: quote.source, updatedAt: quote.updatedAt }
          : market;
      });
      if (portfolio) {
        const marked = markPositionsToMarket(portfolio, quotes);
        if (marked !== portfolio) {
          await savePortfolio(marked);
          portfolio = marked;
        }
      }
      marketsError = '';
    } catch (error) {
      marketsError = error instanceof Error ? error.message : 'Quote refresh failed';
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

  function setActiveView(view: 'landing' | 'desk') {
    activeView = view;
    requestAnimationFrame(() => window.scrollTo(0, 0));
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
      <button class="nav-action" type="button" on:click={() => setActiveView('desk')}>Trading Desk öffnen</button>
    </header>

    <section class="landing-hero" aria-label="KoalaTrade introduction">
      <div class="landing-copy">
        <p class="eyebrow">Virtual trading cockpit</p>
        <h1>Trainiere Märkte, ohne echtes Geld zu riskieren.</h1>
        <p>
          KoalaTrade verbindet Aktien, ETFs, Crypto, Rohstoffe und Event-Märkte in einem schnellen Paper-Trading-Desk.
        </p>
        <div class="landing-actions">
          <button class="primary-button" type="button" on:click={() => setActiveView('desk')}>Desk starten</button>
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
  <main class="trading-shell">
    <header class="trading-topbar">
      <div class="brand">
        <img src="/icons/koalatrade.svg" alt="" width="38" height="38" />
        <div>
          <strong>KoalaTrade</strong>
          <span>Live paper exchange</span>
        </div>
      </div>

      <nav class="desk-tabs" aria-label="Trading sections">
        <button type="button" on:click={() => setActiveView('landing')}><LineChart size={17} /> Home</button>
        <button class="active" type="button"><CandlestickChart size={17} /> Trade</button>
        <button type="button"><WalletCards size={17} /> Portfolio</button>
        <button type="button"><Trophy size={17} /> Seasons</button>
      </nav>

      <div class="desk-actions">
        <span class:online={config} class="status-pill">{config ? 'API online' : 'Local mode'}</span>
        <button class="icon-button" type="button" aria-label="Sync portfolio" title="Sync portfolio" disabled={isSyncing || !portfolio || !!configError} on:click={handleSyncPortfolio}>
          <CloudUpload size={18} />
        </button>
        <button class="icon-button" type="button" aria-label="Reset portfolio" title="Reset portfolio" on:click={handleResetPortfolio}>
          <RotateCcw size={18} />
        </button>
      </div>
    </header>

    <section class="market-tape" aria-label="Market tape">
      {#each markets.slice(0, 6) as item}
        <button class:active={selectedMarket.assetId === item.assetId} type="button" on:click={() => (selectedAssetId = item.assetId)}>
          <strong>{item.symbol}</strong>
          <span>{formatMoney(item.priceCents)}</span>
          <em class={marketTone(item.changeBps)}>{formatPercentFromBps(item.changeBps)}</em>
        </button>
      {/each}
    </section>

    <section class="trade-layout" aria-label="Trading workspace">
      <aside class="watchlist panel" aria-label="Markets">
        <label class="search compact" aria-label="Search markets">
          <Search size={17} />
          <input bind:value={marketQuery} type="search" placeholder="Search markets" />
        </label>
        <div class="watchlist-head"><span>Asset</span><span>Price</span><span>24h</span></div>
        <div class="market-list">
          {#each filteredMarkets as item}
            <button class:selected={selectedMarket.assetId === item.assetId} class="market-row" type="button" on:click={() => (selectedAssetId = item.assetId)}>
              <span><strong>{item.symbol}</strong><small>{item.kind}</small></span>
              <span>{formatMoney(item.priceCents)}</span>
              <em class={marketTone(item.changeBps)}>{formatPercentFromBps(item.changeBps)}</em>
            </button>
          {/each}
        </div>
      </aside>

      <section class="market-stage">
        <section class="instrument-strip panel" aria-label="Selected market">
          <div>
            <p class="eyebrow">{selectedMarket.kind} · {selectedMarket.source}</p>
            <h1>{selectedMarket.symbol}</h1>
            <span>{selectedMarket.name}</span>
          </div>
          <div class="instrument-price">
            <strong>{formatMoney(selectedMarket.priceCents)}</strong>
            <span class={marketTone(selectedMarket.changeBps)}>{formatPercentFromBps(selectedMarket.changeBps)}</span>
          </div>
          <div class="instrument-stats">
            <span>Equity <strong>{formatMoney(summary.totalEquityCents)}</strong></span>
            <span>Cash <strong>{formatMoney(summary.cashCents)}</strong></span>
            <span>Return <strong class:up={summary.totalReturnCents > 0} class:down={summary.totalReturnCents < 0}>{formatPercentFromBps(summary.totalReturnBps)}</strong></span>
          </div>
        </section>

        <section class="chart-panel panel" aria-label="Price chart">
          <div class="chart-toolbar">
            <div><p class="eyebrow">Chart</p><h2>{selectedMarket.symbol} paper market</h2></div>
            <div class="timeframes"><button type="button">1H</button><button class="active" type="button">1D</button><button type="button">1W</button></div>
          </div>
          <div class="chart-surface" aria-hidden="true">
            <span></span><span></span><span></span><span></span><i></i><i></i><i></i><i></i><b></b>
          </div>
        </section>

        <section class="portfolio-dock panel" aria-label="Portfolio and ledger">
          <div class="dock-column">
            <div class="panel-head"><div><p class="eyebrow">Portfolio</p><h2>Exposure</h2></div><WalletCards size={18} /></div>
            <div class="exposure-list">
              {#each exposureRows as row}
                <div class="exposure-row"><span>{row[0]}</span><strong>{row[1]}</strong><em>{row[2]}</em></div>
              {/each}
            </div>
          </div>
          <div class="dock-column">
            <div class="panel-head"><div><p class="eyebrow">Positions</p><h2>Open book</h2></div><Activity size={18} /></div>
            <div class="holding-list">
              {#if positionList.length === 0}
                <p class="empty-state">No open positions yet.</p>
              {:else}
                {#each positionList as position}
                  <div class="holding-row"><strong>{position.symbol}</strong><span>{position.quantity} · {formatMoney(position.lastPriceCents)}</span><em>{position.kind}</em></div>
                {/each}
              {/if}
            </div>
          </div>
          <div class="dock-column">
            <div class="panel-head"><div><p class="eyebrow">Ledger</p><h2>Recent orders</h2></div><ShieldCheck size={18} /></div>
            <div class="ledger-list">
              {#if transactionList.length === 0}
                <p class="empty-state">No simulated trades yet.</p>
              {:else}
                {#each transactionList as transaction}
                  <div class="ledger-row"><strong>{transaction.side.toUpperCase()} {transaction.symbol}</strong><span>{transaction.quantity} @ {formatMoney(transaction.priceCents)}</span><em>{transaction.status}</em></div>
                {/each}
              {/if}
            </div>
          </div>
        </section>
      </section>

      <aside class="execution-column" aria-label="Execution">
        <section class="panel orderbook" aria-label="Order book">
          <div class="panel-head">
            <div><p class="eyebrow">Depth</p><h2>Order book</h2></div>
            <Layers3 size={18} />
          </div>
          <div class="book-head"><span>Price</span><span>Size</span></div>
          <div class="book-list">
            {#each bookRows as row}
              <div class={row.side} style={`--depth:${row.depth}`}>
                <span>{formatMoney(row.priceCents)}</span><strong>{row.size}</strong>
              </div>
            {/each}
          </div>
        </section>

        <section class="order-panel panel" aria-label="Order ticket">
          <div class="panel-head">
            <div><p class="eyebrow">Order ticket</p><h2>{orderSide === 'buy' ? 'Buy' : 'Sell'} {selectedMarket.symbol}</h2></div>
            <CandlestickChart size={18} />
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

        <section class="system-line panel" aria-label="System status">
          {#each systemItems as item}<span>{item}</span>{/each}
          {#if configError}<span class="warning">{configError}</span>{/if}
          {#if portfolioError}<span class="warning">{portfolioError}</span>{/if}
          {#if marketsError}<span class="warning">{marketsError}</span>{/if}
          {#if syncError}<span class="warning">{syncError}</span>{/if}
        </section>
      </aside>
    </section>
  </main>
{/if}
