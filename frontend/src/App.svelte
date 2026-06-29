<script lang="ts">
  import {
    Activity,
    ArrowLeft,
    CandlestickChart,
    CloudUpload,
    Gauge,
    LineChart,
    Keyboard,
    RotateCcw,
    Search,
    ShieldCheck,
    Sparkles,
    TrendingUp,
    Trophy,
    UserCircle2,
    WalletCards,
    Zap
  } from '@lucide/svelte';
  import { onDestroy, onMount } from 'svelte';
  import AdminView from './lib/components/AdminView.svelte';
  import AreaChart from './lib/components/AreaChart.svelte';
  import EsportsView from './lib/components/EsportsView.svelte';
  import OrderBook from './lib/components/OrderBook.svelte';
  import ProfileView from './lib/components/ProfileView.svelte';
  import Sparkline from './lib/components/Sparkline.svelte';
  import Toasts from './lib/components/Toasts.svelte';
  import {
    adminLogin,
    fetchEsportsMatches,
    fetchEsportsResults,
    fetchEsportsTeams,
    fetchMarketHistory,
    fetchMarkets,
    fetchPublicConfig,
    fetchQuotes,
    fetchSyncedPortfolio,
    refreshMatchOdds,
    syncPortfolio,
    type Candle,
    type ChartRange,
    type EsportsMatch,
    type EsportsTeam,
    type EsportsTeamInfo,
    type Market,
    type PublicConfig
  } from './lib/api';
  import { loadClientId, loadPortfolio, loadPreferences, resetPortfolio, savePortfolio, savePreferences } from './lib/portfolio-db';
  import { DEFAULT_LEAGUES, MAX_FAVORITE_TEAMS, defaultPreferences, type Preferences } from './lib/preferences';
  import { toast } from './lib/toast';
  import {
    applyTrade,
    computePerformance,
    createInitialPortfolio,
    formatMoney,
    formatPercentFromBps,
    markPositionsToMarket,
    resolveEventPosition,
    summarizePortfolio,
    PORTFOLIO_ID,
    type PortfolioSnapshot
  } from './lib/portfolio';

  const fallbackMarkets: Market[] = [
    { assetId: 'crypto:btc', symbol: 'BTC', name: 'Bitcoin', kind: 'crypto', source: 'local', priceCents: 6_142_020, changeBps: 280, updatedAt: new Date(0).toISOString() },
    { assetId: 'etf:spy', symbol: 'SPY', name: 'S&P 500 ETF', kind: 'etf', source: 'local', priceCents: 54_618, changeBps: 40, updatedAt: new Date(0).toISOString() },
    { assetId: 'commodity:gld', symbol: 'GLD', name: 'Gold Trust', kind: 'commodity', source: 'local', priceCents: 21_492, changeBps: -20, updatedAt: new Date(0).toISOString() },
    { assetId: 'event:pmkt', symbol: 'PMKT', name: 'Polymarket event markets', kind: 'event', source: 'local', priceCents: 62, changeBps: 0, updatedAt: new Date(0).toISOString() },
    { assetId: 'event:lolesports-t1', symbol: 'LOL-T1', name: 'LoL Esports: T1 match winner', kind: 'event', source: 'local', priceCents: 64, changeBps: 180, updatedAt: new Date(0).toISOString() },
    { assetId: 'event:lolesports-geng', symbol: 'LOL-GEN', name: 'LoL Esports: Gen.G match winner', kind: 'event', source: 'local', priceCents: 41, changeBps: -120, updatedAt: new Date(0).toISOString() }
  ];

  const ORDER_FEE_BPS = 8;
  const QUANTITY_STEP = 0.0001;
  const chartRanges: ChartRange[] = ['1H', '1D', '1W', '1M', '1Y'];
  const quantityPresets = [0.25, 0.5, 0.75, 1] as const;
  const orderTypes = [
    { id: 'market', label: 'Market' },
    { id: 'limit', label: 'Limit' },
    { id: 'stop', label: 'Stop' }
  ] as const;
  type OrderType = (typeof orderTypes)[number]['id'];

  const marketFilters = [
    { id: 'all', label: 'Alle' },
    { id: 'crypto', label: 'Crypto' },
    { id: 'etf', label: 'ETFs' },
    { id: 'commodity', label: 'Metalle' },
    { id: 'event', label: 'eSports' }
  ] as const;
  type MarketFilter = (typeof marketFilters)[number]['id'];

  const deskTabs = [
    { id: 'trade', label: 'Trade', icon: CandlestickChart },
    { id: 'portfolio', label: 'Portfolio', icon: WalletCards },
    { id: 'markets', label: 'Markets', icon: LineChart },
    { id: 'esports', label: 'eSports', icon: Trophy }
  ] as const;
  type DeskView = (typeof deskTabs)[number]['id'];

  let config: PublicConfig | null = null;
  let configError = '';
  let markets: Market[] = fallbackMarkets;
  let marketsError = '';
  let marketsLoading = true;
  let portfolio: PortfolioSnapshot | null = null;
  let portfolioError = '';
  let selectedAssetId = fallbackMarkets[0].assetId;
  let marketQuery = '';
  let marketFilter: MarketFilter = 'all';
  let orderSide: 'buy' | 'sell' = 'buy';
  let orderType: OrderType = 'market';
  let orderQuantity: number | string = 1;
  let limitPriceInput = 0;
  let orderError = '';
  let isSyncing = false;
  let syncMessage = 'Sync bereit';
  type AppView = 'landing' | DeskView | 'profile' | 'admin';
  let activeView: AppView = 'landing';
  const ADMIN_TOKEN_KEY = 'koala-admin-token';
  let adminToken: string | null = null;
  let clientId = '';
  let quoteTimer: ReturnType<typeof setInterval> | undefined;
  let showShortcuts = false;

  // Chart state
  let chartRange: ChartRange = '1D';
  let candles: Candle[] = [];
  let chartLoading = false;
  let showSma = true;
  let historyToken = 0;

  // Portfolio view controls
  let positionSort: 'value' | 'pnl' = 'value';

  // Esports state
  let esportsMatches: EsportsMatch[] = [];
  let esportsLoading = false;
  let esportsError = '';
  let esportsLoaded = false;
  let esportsTeams: EsportsTeamInfo[] = [];
  let teamsLoading = false;
  let teamsLoaded = false;

  // Preferences (favorite teams + default leagues), persisted locally.
  // The profile picker and the eSports page filter share this one source, so
  // changing leagues in either place updates both immediately.
  let preferences: Preferences = defaultPreferences();

  onMount(async () => {
    try {
      preferences = await loadPreferences();
    } catch {
      preferences = defaultPreferences();
    }

    adminToken = localStorage.getItem(ADMIN_TOKEN_KEY);

    try {
      clientId = await loadClientId();
    } catch (error) {
      toast.error('Sync nicht verfügbar', error instanceof Error ? error.message : undefined);
    }

    try {
      config = await fetchPublicConfig();
    } catch (error) {
      configError = error instanceof Error ? error.message : 'Backend nicht erreichbar';
    }

    try {
      markets = await fetchMarkets();
      if (!markets.some((market) => market.assetId === selectedAssetId)) {
        selectedAssetId = markets[0]?.assetId ?? fallbackMarkets[0].assetId;
      }
    } catch (error) {
      marketsError = error instanceof Error ? error.message : 'Marktdaten nicht verfügbar';
      markets = fallbackMarkets;
    } finally {
      marketsLoading = false;
    }

    try {
      portfolio = await loadPortfolio(config?.startingCashCents ?? 1_000_000);
    } catch (error) {
      portfolioError = error instanceof Error ? error.message : 'Lokales Portfolio nicht verfügbar';
      portfolio = createInitialPortfolio(config?.startingCashCents ?? 1_000_000);
    }

    limitPriceInput = selectedMarket.priceCents / 100;
    await restoreSyncedPortfolio();
    await refreshQuotes();
    await loadHistory();
    void settleResolvedBets();
    quoteTimer = setInterval(refreshQuotes, 30_000);
  });

  onDestroy(() => {
    if (quoteTimer) clearInterval(quoteTimer);
  });

  $: summary = summarizePortfolio(portfolio ?? createInitialPortfolio(config?.startingCashCents ?? 1_000_000));
  $: selectedMarket = markets.find((item) => item.assetId === selectedAssetId) ?? markets[0] ?? fallbackMarkets[0];
  $: isEvent = selectedMarket.kind === 'event';
  $: query = marketQuery.trim().toLowerCase();
  $: filteredMarkets = markets.filter((market) => {
    const matchesFilter = marketFilter === 'all' || market.kind === marketFilter;
    const matchesQuery = !query || `${market.symbol} ${market.name} ${market.kind}`.toLowerCase().includes(query);
    return matchesFilter && matchesQuery;
  });

  $: effectivePriceCents =
    orderType === 'market' ? selectedMarket.priceCents : Math.max(0, Math.round(Number(limitPriceInput) * 100));
  $: normalizedOrderQuantity = Number.isFinite(Number(orderQuantity)) ? Number(orderQuantity) : 0;
  $: selectedPosition = positionList.find((position) => position.assetId === selectedMarket.assetId);
  $: selectedPositionQuantity = selectedPosition?.quantity ?? 0;
  $: estimatedOrderValue = Math.round(normalizedOrderQuantity * effectivePriceCents);
  $: estimatedOrderFee = Math.max(0, Math.round((estimatedOrderValue * ORDER_FEE_BPS) / 10_000));
  $: estimatedOrderTotal = orderSide === 'buy' ? estimatedOrderValue + estimatedOrderFee : Math.max(0, estimatedOrderValue - estimatedOrderFee);
  $: maxBuyQuantity = effectivePriceCents > 0 ? roundQuantity(summary.cashCents / (effectivePriceCents * (1 + ORDER_FEE_BPS / 10_000))) : 0;
  $: maxSellQuantity = roundQuantity(selectedPositionQuantity);
  $: orderLimitQuantity = orderSide === 'buy' ? maxBuyQuantity : maxSellQuantity;
  $: canSubmitOrder =
    !!portfolio &&
    effectivePriceCents > 0 &&
    normalizedOrderQuantity > 0 &&
    normalizedOrderQuantity <= orderLimitQuantity &&
    (orderSide === 'buy' ? estimatedOrderTotal <= summary.cashCents : selectedPositionQuantity >= normalizedOrderQuantity);
  $: orderPowerLabel = orderSide === 'buy' ? 'Kaufkraft' : 'Verfügbar';
  $: orderPowerValue = orderSide === 'buy' ? formatMoney(summary.cashCents) : `${formatQuantity(selectedPositionQuantity)} ${selectedMarket.symbol}`;
  $: positionList = portfolio?.positions ?? [];
  $: transactionList = portfolio?.transactions.slice(0, 6) ?? [];
  $: positionRows = positionList.map((position) => {
    const marketValueCents = Math.round(position.quantity * position.lastPriceCents);
    const costBasisCents = Math.round(position.quantity * position.averageCostCents);
    const pnlCents = marketValueCents - costBasisCents;
    const pnlBps = costBasisCents > 0 ? Math.round((pnlCents / costBasisCents) * 10_000) : 0;
    return { ...position, marketValueCents, pnlCents, pnlBps };
  });
  $: sortedPositionRows = [...positionRows].sort((a, b) =>
    positionSort === 'pnl' ? b.pnlCents - a.pnlCents : b.marketValueCents - a.marketValueCents
  );
  $: performance = computePerformance(
    portfolio ?? createInitialPortfolio(config?.startingCashCents ?? 1_000_000),
    summary.totalEquityCents
  );

  // Price chart series
  $: closes = candles.map((candle) => candle.close);
  $: chartLabels = candles.map((candle) => candle.time);
  $: smaSeries = showSma && closes.length > 14 ? simpleMovingAverage(closes, 14) : null;
  $: chartHigh = candles.length ? Math.max(...candles.map((c) => c.high)) : selectedMarket.priceCents;
  $: chartLow = candles.length ? Math.min(...candles.map((c) => c.low)) : selectedMarket.priceCents;
  $: chartOpen = candles.length ? candles[0].open : selectedMarket.priceCents;
  $: rangeChangeCents = candles.length ? selectedMarket.priceCents - candles[0].open : 0;
  $: rangeChangeBps = chartOpen > 0 ? Math.round((rangeChangeCents / chartOpen) * 10_000) : 0;

  // React to chart input changes once data is loaded.
  $: if (activeView !== 'landing' && selectedAssetId && chartRange) {
    void loadHistory();
  }

  // Lazy-load esports matches the first time the tab is opened.
  $: if (activeView === 'esports' && !esportsLoaded && !esportsLoading) {
    void loadEsports();
  }

  // Lazy-load the team catalogue when the profile is opened (favorites picker).
  $: if (activeView === 'profile' && !teamsLoaded && !teamsLoading) {
    void loadTeams();
  }

  // Load matches for the admin "no odds" diagnostic when the admin panel opens.
  $: if (activeView === 'admin' && adminToken && !esportsLoaded && !esportsLoading) {
    void loadEsports();
  }

  // League toggle options: curated defaults + whatever leagues are live right now.
  $: leagueOptions = Array.from(
    new Set([...DEFAULT_LEAGUES, ...preferences.esportsLeagues, ...esportsMatches.map((match) => match.league).filter(Boolean)])
  );

  async function loadHistory() {
    if (marketsLoading) return;
    const token = ++historyToken;
    chartLoading = candles.length === 0;
    try {
      const next = await fetchMarketHistory(selectedAssetId, chartRange);
      if (token === historyToken) candles = next;
    } catch {
      if (token === historyToken) candles = [];
    } finally {
      if (token === historyToken) chartLoading = false;
    }
  }

  async function loadEsports() {
    esportsLoading = true;
    esportsError = '';
    try {
      esportsMatches = await fetchEsportsMatches();
      esportsLoaded = true;
      await reconcileEsportsPositions();
      await settleResolvedBets();
    } catch (error) {
      esportsError = error instanceof Error ? error.message : 'eSports-Feed nicht erreichbar';
    } finally {
      esportsLoading = false;
    }
  }

  function parseEsportsAsset(assetId: string): { matchId: string; teamCode: string } | null {
    const parts = assetId.split(':'); // event:lol:<matchId>:<teamCode>
    if (parts.length < 4 || parts[0] !== 'event' || parts[1] !== 'lol') return null;
    return { matchId: parts[2], teamCode: parts[3] };
  }

  // Auto-resolve open bets whose match has completed: winning Yes pays 100¢,
  // losing Yes settles at 0¢, both credited automatically.
  async function settleResolvedBets() {
    if (!portfolio) return;
    const held = portfolio.positions.filter((position) => position.assetId.startsWith('event:lol:'));
    if (held.length === 0) return;

    const matchIds = Array.from(new Set(held.map((position) => parseEsportsAsset(position.assetId)?.matchId).filter((id): id is string => !!id)));
    let results;
    try {
      results = await fetchEsportsResults(matchIds);
    } catch {
      return;
    }
    if (results.length === 0) return;

    const byMatch = new Map(results.map((result) => [result.matchId, result]));
    let snapshot = portfolio;
    const settled: { symbol: string; won: boolean; quantity: number }[] = [];

    for (const position of held) {
      const parsed = parseEsportsAsset(position.assetId);
      if (!parsed) continue;
      const result = byMatch.get(parsed.matchId);
      if (!result) continue;
      const won = result.winnerCode.toUpperCase() === parsed.teamCode.toUpperCase();
      const next = resolveEventPosition(snapshot, position.assetId, won);
      if (next) {
        snapshot = next;
        settled.push({ symbol: position.symbol, won, quantity: position.quantity });
      }
    }

    if (settled.length > 0) {
      await savePortfolio(snapshot);
      portfolio = snapshot;
      for (const item of settled) {
        if (item.won) {
          toast.success('Wette gewonnen', `${item.symbol} → +${formatMoney(item.quantity * 100)}`);
        } else {
          toast.info('Wette verloren', `${item.symbol} ist wertlos verfallen.`);
        }
      }
    }
  }

  async function buyMoreEsports(assetId: string, contracts: number) {
    const parsed = parseEsportsAsset(assetId);
    if (!parsed) return;
    let match = esportsMatches.find((item) => item.id === parsed.matchId);
    if (!match) {
      toast.error('Markt nicht verfügbar', 'Dieses Match ist aktuell nicht handelbar.');
      return;
    }
    await handleRefreshOdds(parsed.matchId);
    match = esportsMatches.find((item) => item.id === parsed.matchId);
    if (!match) return;
    const team = match.team1.code === parsed.teamCode ? match.team1 : match.team2.code === parsed.teamCode ? match.team2 : null;
    if (!team || team.priceCents <= 0) {
      toast.error('Keine aktuelle Quote', 'Für dieses Team gibt es gerade keine Quote.');
      return;
    }
    await placeEsportsBet(match, team, contracts);
  }

  async function handleAdminLogin(username: string, password: string) {
    const { token } = await adminLogin(username, password);
    adminToken = token;
    localStorage.setItem(ADMIN_TOKEN_KEY, token);
    toast.success('Angemeldet', 'Admin-Bereich entsperrt.');
  }

  function handleAdminLogout() {
    adminToken = null;
    localStorage.removeItem(ADMIN_TOKEN_KEY);
  }

  async function loadTeams() {
    teamsLoading = true;
    try {
      esportsTeams = await fetchEsportsTeams();
      teamsLoaded = true;
    } catch {
      // Non-fatal: the favorites picker just stays empty.
    } finally {
      teamsLoading = false;
    }
  }

  async function persistPreferences() {
    try {
      await savePreferences(preferences);
    } catch {
      // Local persistence is best-effort.
    }
  }

  function toggleFavoriteTeam(code: string) {
    const favorites = preferences.favoriteTeams;
    if (favorites.includes(code)) {
      preferences = { ...preferences, favoriteTeams: favorites.filter((c) => c !== code) };
    } else if (favorites.length < MAX_FAVORITE_TEAMS) {
      preferences = { ...preferences, favoriteTeams: [...favorites, code] };
    } else {
      toast.info('Limit erreicht', `Maximal ${MAX_FAVORITE_TEAMS} Lieblingsteams.`);
      return;
    }
    void persistPreferences();
  }

  function toggleDefaultLeague(league: string) {
    const leagues = preferences.esportsLeagues;
    preferences = {
      ...preferences,
      esportsLeagues: leagues.includes(league) ? leagues.filter((l) => l !== league) : [...leagues, league]
    };
    void persistPreferences();
  }


  // Force-refresh one match's odds from Polymarket (no rate limit) before betting.
  async function handleRefreshOdds(matchId: string) {
    try {
      const fresh = await refreshMatchOdds(matchId);
      esportsMatches = esportsMatches.map((match) => (match.id === matchId ? fresh : match));
      await reconcileEsportsPositions();
    } catch {
      // Keep the previously shown odds if the refresh fails.
    }
  }

  // Re-price held esports bet positions to the latest Polymarket odds.
  async function reconcileEsportsPositions() {
    if (!portfolio || esportsMatches.length === 0) return;
    const priceByAsset = new Map<string, number>();
    for (const match of esportsMatches) {
      if (!match.hasOdds) continue;
      for (const team of [match.team1, match.team2]) {
        if (team.priceCents > 0) priceByAsset.set(esportsAssetId(match.id, team.code), team.priceCents);
      }
    }
    const updates = portfolio.positions
      .filter((position) => priceByAsset.has(position.assetId))
      .map((position) => ({ assetId: position.assetId, priceCents: priceByAsset.get(position.assetId)! }));
    if (updates.length === 0) return;
    const marked = markPositionsToMarket(portfolio, updates);
    if (marked !== portfolio) {
      await savePortfolio(marked);
      portfolio = marked;
    }
  }

  function esportsAssetId(matchId: string, teamCode: string) {
    return `event:lol:${matchId}:${teamCode}`;
  }

  async function placeEsportsBet(match: EsportsMatch, team: EsportsTeam, contracts: number) {
    if (!portfolio) return;
    const other = team.code === match.team1.code ? match.team2 : match.team1;
    const grossCents = Math.round(contracts * team.priceCents);
    const feeCents = Math.max(0, Math.round((grossCents * ORDER_FEE_BPS) / 10_000));
    try {
      const next = applyTrade(portfolio, {
        id: crypto.randomUUID(),
        assetId: esportsAssetId(match.id, team.code),
        symbol: team.code,
        name: `${team.name} schlägt ${other.code} · ${match.league}`,
        kind: 'event',
        side: 'buy',
        quantity: contracts,
        priceCents: team.priceCents,
        feeCents
      });
      await savePortfolio(next);
      portfolio = next;
      toast.success('Wette platziert', `${contracts}× ${team.code} @ ${formatMoney(team.priceCents)}`);
    } catch (error) {
      toast.error('Wette fehlgeschlagen', error instanceof Error ? error.message : undefined);
    }
  }

  async function sellEsportsPosition(assetId: string, quantity: number) {
    if (!portfolio) return;
    const position = portfolio.positions.find((item) => item.assetId === assetId);
    if (!position) return;
    const grossCents = Math.round(quantity * position.lastPriceCents);
    const feeCents = Math.max(0, Math.round((grossCents * ORDER_FEE_BPS) / 10_000));
    try {
      const next = applyTrade(portfolio, {
        id: crypto.randomUUID(),
        assetId: position.assetId,
        symbol: position.symbol,
        name: position.name,
        kind: position.kind,
        side: 'sell',
        quantity,
        priceCents: position.lastPriceCents,
        feeCents
      });
      await savePortfolio(next);
      portfolio = next;
      toast.success('Cash-out', `${position.symbol} für ${formatMoney(grossCents - feeCents)}`);
    } catch (error) {
      toast.error('Cash-out fehlgeschlagen', error instanceof Error ? error.message : undefined);
    }
  }

  async function handleResetPortfolio() {
    portfolio = await resetPortfolio(config?.startingCashCents ?? portfolio?.startingCashCents ?? 1_000_000);
    orderError = '';
    toast.info('Portfolio zurückgesetzt', 'Startkapital wiederhergestellt.');
  }

  async function handleSubmitOrder() {
    orderError = '';
    if (!portfolio) {
      orderError = 'Portfolio wird noch geladen';
      return;
    }
    if (effectivePriceCents <= 0) {
      orderError = 'Bitte einen gültigen Limit-Preis eingeben';
      return;
    }
    if (!canSubmitOrder) {
      orderError = orderSide === 'buy' ? 'Nicht genug Kaufkraft für diese Order' : 'Nicht genug verfügbare Position';
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
        priceCents: effectivePriceCents,
        feeCents: estimatedOrderFee
      });
      await savePortfolio(nextPortfolio);
      portfolio = nextPortfolio;
      toast.success(
        `${orderSide === 'buy' ? 'Kauf' : 'Verkauf'} ausgeführt`,
        `${formatQuantity(normalizedOrderQuantity)} ${selectedMarket.symbol} @ ${formatMoney(effectivePriceCents)}`
      );
    } catch (error) {
      orderError = error instanceof Error ? error.message : 'Order konnte nicht platziert werden';
      toast.error('Order fehlgeschlagen', orderError);
    }
  }

  async function handleSyncPortfolio() {
    if (!portfolio) return;
    isSyncing = true;
    try {
      const synced = await syncPortfolio(clientId || (await loadClientId()), portfolio);
      await savePortfolio(synced, { touchUpdatedAt: false });
      portfolio = synced;
      syncMessage = `Synchronisiert ${formatUpdatedAt(synced.updatedAt)}`;
      toast.success('Portfolio synchronisiert');
    } catch (error) {
      syncMessage = 'Sync fehlgeschlagen';
      toast.error('Sync fehlgeschlagen', error instanceof Error ? error.message : undefined);
    } finally {
      isSyncing = false;
    }
  }

  async function restoreSyncedPortfolio() {
    if (!portfolio || !clientId || configError) return;
    try {
      const synced = await fetchSyncedPortfolio(clientId, PORTFOLIO_ID);
      if (!synced) {
        syncMessage = 'Sync bereit';
        return;
      }
      if (new Date(synced.updatedAt).getTime() > new Date(portfolio.updatedAt).getTime()) {
        await savePortfolio(synced, { touchUpdatedAt: false });
        portfolio = synced;
        syncMessage = `Wiederhergestellt ${formatUpdatedAt(synced.updatedAt)}`;
        return;
      }
      syncMessage = 'Lokales Portfolio aktuell';
    } catch {
      syncMessage = 'Sync nicht verfügbar';
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
      marketsError = error instanceof Error ? error.message : 'Quote-Aktualisierung fehlgeschlagen';
    }
  }

  function simpleMovingAverage(values: number[], period: number) {
    const result: number[] = [];
    for (let i = 0; i < values.length; i++) {
      const start = Math.max(0, i - period + 1);
      const window = values.slice(start, i + 1);
      result.push(window.reduce((sum, value) => sum + value, 0) / window.length);
    }
    return result;
  }

  function miniSeries(market: Market): number[] {
    const n = 18;
    const base = market.priceCents || 1;
    const drift = market.changeBps / 10_000;
    const start = base / (1 + drift || 1);
    let seed = 0;
    for (const ch of market.assetId) seed = (seed * 31 + ch.charCodeAt(0)) >>> 0;
    const out: number[] = [];
    for (let i = 0; i < n; i++) {
      seed = (seed * 1_103_515_245 + 12_345) & 0x7fffffff;
      const t = i / (n - 1);
      const noise = (seed / 0x7fffffff - 0.5) * (Math.abs(drift) || 0.01) * base * 0.55;
      out.push(start + (base - start) * t + noise);
    }
    out[n - 1] = base;
    return out;
  }

  function resolutionLabel(market: Market) {
    let seed = 0;
    for (const ch of market.assetId) seed = (seed * 17 + ch.charCodeAt(0)) >>> 0;
    const days = 2 + (seed % 21);
    const date = new Date();
    date.setDate(date.getDate() + days);
    return { days, label: new Intl.DateTimeFormat('de-DE', { day: '2-digit', month: 'short' }).format(date) };
  }

  function formatUpdatedAt(value: string) {
    return new Intl.DateTimeFormat('de-DE', { hour: '2-digit', minute: '2-digit' }).format(new Date(value));
  }

  function formatChartTime(value: string) {
    const date = new Date(value);
    if (chartRange === '1H' || chartRange === '1D') {
      return new Intl.DateTimeFormat('de-DE', { hour: '2-digit', minute: '2-digit' }).format(date);
    }
    return new Intl.DateTimeFormat('de-DE', { day: '2-digit', month: 'short' }).format(date);
  }

  function formatQuantity(value: number) {
    return new Intl.NumberFormat('de-DE', { maximumFractionDigits: 6 }).format(value);
  }

  function formatSignedMoney(cents: number) {
    return `${cents > 0 ? '+' : cents < 0 ? '−' : ''}${formatMoney(Math.abs(cents))}`;
  }

  function roundQuantity(value: number) {
    return Math.max(0, Math.floor(value / QUANTITY_STEP) * QUANTITY_STEP);
  }

  function setOrderSide(side: 'buy' | 'sell') {
    orderSide = side;
    orderError = '';
  }

  function applyPreset(fraction: number) {
    const value = roundQuantity(orderLimitQuantity * fraction);
    orderQuantity = value || QUANTITY_STEP;
    orderError = '';
  }

  function selectMarket(assetId: string, jumpToTrade = false) {
    selectedAssetId = assetId;
    orderError = '';
    const market = markets.find((item) => item.assetId === assetId);
    if (market) limitPriceInput = market.priceCents / 100;
    if (jumpToTrade) setActiveView('trade');
  }

  function marketTone(changeBps: number) {
    if (changeBps > 0) return 'up';
    if (changeBps < 0) return 'down';
    return 'flat';
  }

  function setActiveView(view: AppView) {
    activeView = view;
    requestAnimationFrame(() => window.scrollTo(0, 0));
  }

  function handleKeydown(event: KeyboardEvent) {
    if (activeView === 'landing') return;
    const target = event.target as HTMLElement | null;
    if (target && ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)) return;
    if (event.metaKey || event.ctrlKey || event.altKey) return;

    const key = event.key.toLowerCase();
    if (key === 'b' && activeView === 'trade') {
      setOrderSide('buy');
    } else if (key === 's' && activeView === 'trade') {
      setOrderSide('sell');
    } else if (key === '?') {
      showShortcuts = !showShortcuts;
    } else if (/^[1-6]$/.test(event.key)) {
      const market = markets[Number(event.key) - 1];
      if (market) selectMarket(market.assetId);
    }
  }

  function changeColor(bps: number) {
    return bps > 0 ? 'up' : bps < 0 ? 'down' : 'flat';
  }
</script>

<svelte:window on:keydown={handleKeydown} />
<Toasts />

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
      <button class="nav-action" type="button" on:click={() => setActiveView('trade')}>Trading Desk öffnen</button>
    </header>

    <section class="landing-hero" aria-label="KoalaTrade introduction">
      <div class="landing-copy">
        <p class="eyebrow"><Sparkles size={14} /> Virtuelles Trading-Cockpit</p>
        <h1>Märkte meistern, ohne echtes Geld zu riskieren.</h1>
        <p>
          KoalaTrade vereint Aktien, ETFs, Crypto, Rohstoffe und eSports-Eventmärkte in einem schnellen Paper-Trading-Desk —
          zum Lernen, Üben und Wettbewerben.
        </p>
        <div class="landing-actions">
          <button class="primary-button" type="button" on:click={() => setActiveView('trade')}>Desk starten</button>
          <span class:online={config}>{config ? 'Live API bereit' : 'Lädt lokale Session'}</span>
        </div>
        <div class="landing-stats">
          <div><strong>{markets.length}</strong><span>Märkte</span></div>
          <div><strong>{formatMoney(summary.totalEquityCents)}</strong><span>Equity</span></div>
          <div><strong>0 €</strong><span>Risiko</span></div>
        </div>
      </div>

      <div class="landing-terminal" aria-label="Product preview">
        <div class="preview-marketbar">
          <div>
            <strong>{selectedMarket.symbol}</strong>
            <span>{selectedMarket.name}</span>
          </div>
          <strong>{formatMoney(selectedMarket.priceCents)}</strong>
          <em class={marketTone(selectedMarket.changeBps)}>{formatPercentFromBps(selectedMarket.changeBps)}</em>
        </div>

        <div class="preview-chart">
          <AreaChart
            series={candles.length ? candles.map((c) => c.close) : miniSeries(selectedMarket)}
            height={210}
            formatValue={formatMoney}
          />
        </div>

        <div class="preview-side">
          <div><span>Equity</span><strong>{formatMoney(summary.totalEquityCents)}</strong></div>
          <div><span>Cash</span><strong>{formatMoney(summary.cashCents)}</strong></div>
          <div><span>Return</span><strong class={changeColor(summary.totalReturnBps)}>{formatPercentFromBps(summary.totalReturnBps)}</strong></div>
        </div>
      </div>
    </section>

    <section class="landing-bands" aria-label="Core product areas">
      <article>
        <CandlestickChart size={20} />
        <strong>Multi-Asset Watchlist</strong>
        <span>Ein normalisiertes Marktmodell für Crypto, ETFs, Rohstoffe und Eventmärkte.</span>
      </article>
      <article>
        <WalletCards size={20} />
        <strong>Portfolio-Analytics</strong>
        <span>Equity-Kurve, realisierter & unrealisierter P&L, Drawdown und Order-History.</span>
      </article>
      <article>
        <TrendingUp size={20} />
        <strong>eSports-Märkte</strong>
        <span>LoL-Eventmärkte mit Yes/No-Ansicht und Auflösungsdatum.</span>
      </article>
    </section>
  </main>
{:else}
  <main class="trading-shell">
    <header class="trading-topbar">
      <div class="brand">
        <img src="/icons/koalatrade.svg" alt="" width="34" height="34" />
        <div>
          <strong>KoalaTrade</strong>
          <span>Live paper exchange</span>
        </div>
      </div>

      <nav class="desk-tabs" aria-label="Trading sections">
        {#each deskTabs as tab}
          <button class:active={activeView === tab.id} type="button" on:click={() => setActiveView(tab.id)}>
            <svelte:component this={tab.icon} size={16} /> {tab.label}
          </button>
        {/each}
      </nav>

      <div class="desk-actions">
        <span class:online={config && !configError} class="status-pill">
          <i class="dot"></i>{config && !configError ? 'API online' : 'Local mode'}
        </span>
        <button class="icon-button" class:active={activeView === 'profile'} type="button" aria-label="Profil" title="Profil & Favoriten" on:click={() => setActiveView('profile')}>
          <UserCircle2 size={18} />
        </button>
        <button class="icon-button" class:active={activeView === 'admin'} type="button" aria-label="Admin" title="Admin" on:click={() => setActiveView('admin')}>
          <ShieldCheck size={18} />
        </button>
        <button class="icon-button" type="button" aria-label="Shortcuts" title="Tastenkürzel (?)" on:click={() => (showShortcuts = !showShortcuts)}>
          <Keyboard size={18} />
        </button>
        <button class="icon-button" type="button" aria-label="Portfolio synchronisieren" title="Sync" disabled={isSyncing || !portfolio || !!configError} on:click={handleSyncPortfolio}>
          <CloudUpload size={18} />
        </button>
        <button class="icon-button" type="button" aria-label="Portfolio zurücksetzen" title="Reset" on:click={handleResetPortfolio}>
          <RotateCcw size={18} />
        </button>
      </div>
    </header>

    <section class="market-tape" aria-label="Market tape">
      {#each markets.slice(0, 6) as item, index}
        <button class:active={selectedMarket.assetId === item.assetId} type="button" on:click={() => selectMarket(item.assetId, true)}>
          <span class="tape-key">{index + 1}</span>
          <strong>{item.symbol}</strong>
          <span class="tape-price">{formatMoney(item.priceCents)}</span>
          <em class={marketTone(item.changeBps)}>{formatPercentFromBps(item.changeBps)}</em>
        </button>
      {/each}
    </section>

    {#if activeView === 'trade'}
      <section class="trade-layout" aria-label="Trading workspace">
        <aside class="watchlist panel" aria-label="Markets">
          <label class="search compact" aria-label="Märkte durchsuchen">
            <Search size={16} />
            <input bind:value={marketQuery} type="search" placeholder="Märkte durchsuchen" />
          </label>
          <div class="market-filters" aria-label="Markt-Filter">
            {#each marketFilters as filter}
              <button class:active={marketFilter === filter.id} type="button" on:click={() => (marketFilter = filter.id)}>{filter.label}</button>
            {/each}
          </div>
          <div class="watchlist-head"><span>Asset</span><span>Preis</span><span>24h</span></div>
          <div class="market-list">
            {#if marketsLoading}
              {#each Array(6) as _}<div class="skeleton-row"></div>{/each}
            {:else if filteredMarkets.length === 0}
              <p class="empty-state">Keine Märkte für diesen Filter.</p>
            {:else}
              {#each filteredMarkets as item}
                <button class:selected={selectedMarket.assetId === item.assetId} class="market-row" type="button" on:click={() => selectMarket(item.assetId)}>
                  <span class="asset"><strong>{item.symbol}</strong><small>{item.kind}</small></span>
                  <span class="price">{formatMoney(item.priceCents)}</span>
                  <em class={marketTone(item.changeBps)}>{formatPercentFromBps(item.changeBps)}</em>
                </button>
              {/each}
            {/if}
          </div>
        </aside>

        <section class="market-stage">
          <section class="instrument-strip panel" aria-label="Selected market">
            <div class="instrument-id">
              <p class="eyebrow">{selectedMarket.kind} · {selectedMarket.source}</p>
              <h1>{selectedMarket.symbol}</h1>
              <span>{selectedMarket.name}</span>
            </div>
            <div class="instrument-price">
              <strong>{formatMoney(selectedMarket.priceCents)}</strong>
              <span class={marketTone(selectedMarket.changeBps)}>{formatPercentFromBps(selectedMarket.changeBps)} heute</span>
            </div>
            <div class="instrument-stats">
              <span>Equity <strong>{formatMoney(summary.totalEquityCents)}</strong></span>
              <span>Cash <strong>{formatMoney(summary.cashCents)}</strong></span>
              <span>Return <strong class={changeColor(summary.totalReturnBps)}>{formatPercentFromBps(summary.totalReturnBps)}</strong></span>
            </div>
          </section>

          <section class="chart-panel panel" aria-label="Price chart">
            <div class="chart-toolbar">
              <div>
                <p class="eyebrow">Chart · {chartRange}</p>
                <h2>{formatMoney(selectedMarket.priceCents)} <em class={changeColor(rangeChangeBps)}>{formatSignedMoney(rangeChangeCents)} ({formatPercentFromBps(rangeChangeBps)})</em></h2>
              </div>
              <div class="chart-controls">
                <button class="sma-toggle" class:active={showSma} type="button" on:click={() => (showSma = !showSma)}>SMA 14</button>
                <div class="timeframes">
                  {#each chartRanges as range}
                    <button class:active={chartRange === range} type="button" on:click={() => (chartRange = range)}>{range}</button>
                  {/each}
                </div>
              </div>
            </div>
            <AreaChart
              series={closes}
              labels={chartLabels}
              overlay={smaSeries}
              loading={chartLoading}
              height={300}
              formatValue={formatMoney}
              formatLabel={formatChartTime}
            />
            <div class="chart-stats">
              <span>Open <strong>{formatMoney(chartOpen)}</strong></span>
              <span>Hoch <strong>{formatMoney(chartHigh)}</strong></span>
              <span>Tief <strong>{formatMoney(chartLow)}</strong></span>
              <span>Spanne <strong>{formatPercentFromBps(chartLow > 0 ? Math.round(((chartHigh - chartLow) / chartLow) * 10_000) : 0)}</strong></span>
            </div>
          </section>
        </section>

        <aside class="execution-column" aria-label="Execution">
          {#if isEvent}
            <section class="panel event-card" aria-label="Event market">
              <div class="panel-head"><div><p class="eyebrow">Prediction</p><h2>Wahrscheinlichkeit</h2></div><Gauge size={18} /></div>
              <div class="event-prob">
                <div class="prob-yes" style={`--p:${Math.min(100, Math.round(selectedMarket.priceCents))}%`}>
                  <span>Yes</span><strong>{Math.round(selectedMarket.priceCents)}%</strong>
                </div>
                <div class="prob-no" style={`--p:${Math.max(0, 100 - Math.round(selectedMarket.priceCents))}%`}>
                  <span>No</span><strong>{Math.max(0, 100 - Math.round(selectedMarket.priceCents))}%</strong>
                </div>
              </div>
              <div class="event-meta">
                <span>Auflösung <strong>{resolutionLabel(selectedMarket).label}</strong></span>
                <span>in <strong>{resolutionLabel(selectedMarket).days} Tagen</strong></span>
              </div>
              <p class="event-hint">Yes-Kontrakt zu {formatMoney(selectedMarket.priceCents)} · Auszahlung {formatMoney(10_000)} bei Win.</p>
            </section>
          {:else}
            <section class="panel orderbook" aria-label="Order book">
              <div class="panel-head"><div><p class="eyebrow">Tiefe</p><h2>Orderbuch</h2></div><Activity size={18} /></div>
              <OrderBook priceCents={selectedMarket.priceCents} symbol={selectedMarket.symbol} />
            </section>
          {/if}

          <section class="order-panel panel" aria-label="Order ticket">
            <div class="panel-head">
              <div><p class="eyebrow">Order-Ticket</p><h2>{isEvent ? (orderSide === 'buy' ? 'Buy Yes' : 'Sell Yes') : orderSide === 'buy' ? 'Kaufen' : 'Verkaufen'} {selectedMarket.symbol}</h2></div>
              <Zap size={18} />
            </div>
            <form class="order-form" on:submit|preventDefault={handleSubmitOrder}>
              <div class="segmented" aria-label="Order-Seite">
                <button class:active={orderSide === 'buy'} type="button" on:click={() => setOrderSide('buy')}>{isEvent ? 'Yes' : 'Kaufen'}</button>
                <button class:active={orderSide === 'sell'} class="sell" type="button" on:click={() => setOrderSide('sell')}>{isEvent ? 'No' : 'Verkaufen'}</button>
              </div>

              <div class="order-types" aria-label="Order-Typ">
                {#each orderTypes as type}
                  <button class:active={orderType === type.id} type="button" on:click={() => (orderType = type.id)}>{type.label}</button>
                {/each}
              </div>

              {#if orderType !== 'market'}
                <label class="field">
                  <span>{orderType === 'limit' ? 'Limit-Preis' : 'Stop-Preis'} ($)</span>
                  <input bind:value={limitPriceInput} min="0" step="0.01" type="number" />
                </label>
              {/if}

              <label class="field">
                <span>Menge</span>
                <input bind:value={orderQuantity} min="0.0001" step="0.0001" type="number" />
              </label>

              <div class="presets" aria-label="Mengen-Presets">
                {#each quantityPresets as preset}
                  <button type="button" disabled={orderLimitQuantity <= 0} on:click={() => applyPreset(preset)}>{Math.round(preset * 100)}%</button>
                {/each}
              </div>

              <div class="order-power"><span>{orderPowerLabel}</span><strong>{orderPowerValue}</strong></div>

              <div class="order-summary">
                <div><span>{orderType === 'market' ? 'Marktpreis' : orderType === 'limit' ? 'Limit-Preis' : 'Stop-Preis'}</span><strong>{formatMoney(effectivePriceCents)}</strong></div>
                <div><span>Bruttowert</span><strong>{formatMoney(estimatedOrderValue)}</strong></div>
                <div><span>Gebühr ({(ORDER_FEE_BPS / 100).toFixed(2)}%)</span><strong>{formatMoney(estimatedOrderFee)}</strong></div>
                <div class="total"><span>{orderSide === 'buy' ? 'Cash-Belastung' : 'Cash-Gutschrift'}</span><strong>{formatMoney(estimatedOrderTotal)}</strong></div>
              </div>

              {#if orderError}<p class="form-error">{orderError}</p>{/if}
              <button class="primary-button" class:danger={orderSide === 'sell'} type="submit" disabled={!canSubmitOrder}>
                {isEvent ? (orderSide === 'buy' ? 'Buy Yes' : 'Sell Yes') : orderSide === 'buy' ? 'Kaufen' : 'Verkaufen'} {selectedMarket.symbol}
              </button>
            </form>
          </section>
        </aside>
      </section>
    {:else if activeView === 'portfolio'}
      <section class="view-scroll" aria-label="Portfolio">
        <section class="portfolio-metrics">
          <div class="metric primary">
            <span>Equity</span>
            <strong>{formatMoney(summary.totalEquityCents)}</strong>
            <em class={changeColor(summary.totalReturnBps)}>{formatSignedMoney(summary.totalReturnCents)} ({formatPercentFromBps(summary.totalReturnBps)})</em>
          </div>
          <div class="metric"><span>Cash</span><strong>{formatMoney(summary.cashCents)}</strong><em>{summary.openPositions} Positionen</em></div>
          <div class="metric"><span>Realisierter P&L</span><strong class={changeColor(performance.realizedPnlCents)}>{formatSignedMoney(performance.realizedPnlCents)}</strong><em>geschlossen</em></div>
          <div class="metric"><span>Unrealisiert</span><strong class={changeColor(performance.unrealizedPnlCents)}>{formatSignedMoney(performance.unrealizedPnlCents)}</strong><em>offen</em></div>
          <div class="metric"><span>Max Drawdown</span><strong class:down={performance.drawdownBps > 0}>{formatPercentFromBps(-performance.drawdownBps)}</strong><em>Peak {formatMoney(performance.peakEquityCents)}</em></div>
        </section>

        <section class="panel" aria-label="Equity curve">
          <div class="panel-head"><div><p class="eyebrow">Performance</p><h2>Equity-Kurve</h2></div><LineChart size={18} /></div>
          <AreaChart
            series={performance.curve.map((point) => point.equityCents)}
            labels={performance.curve.map((point) => point.t)}
            height={260}
            accent={summary.totalReturnCents >= 0 ? 'up' : 'down'}
            formatValue={formatMoney}
            formatLabel={(value) => new Intl.DateTimeFormat('de-DE', { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' }).format(new Date(value))}
          />
        </section>

        <div class="portfolio-grid">
          <section class="panel" aria-label="Positions">
            <div class="panel-head">
              <div><p class="eyebrow">Holdings</p><h2>Positionen</h2></div>
              <div class="mini-toggle">
                <button class:active={positionSort === 'value'} type="button" on:click={() => (positionSort = 'value')}>Wert</button>
                <button class:active={positionSort === 'pnl'} type="button" on:click={() => (positionSort = 'pnl')}>P&L</button>
              </div>
            </div>
            <div class="table">
              <div class="table-head pos"><span>Asset</span><span>Menge</span><span>Wert</span><span>P&L</span></div>
              {#if sortedPositionRows.length === 0}
                <p class="empty-state">Noch keine offenen Positionen.</p>
              {:else}
                {#each sortedPositionRows as position}
                  <button class="table-row pos" type="button" on:click={() => selectMarket(position.assetId, true)}>
                    <span class="asset"><strong>{position.symbol}</strong><small>Ø {formatMoney(position.averageCostCents)}</small></span>
                    <span>{formatQuantity(position.quantity)}</span>
                    <span>{formatMoney(position.marketValueCents)}</span>
                    <em class={changeColor(position.pnlCents)}>{formatSignedMoney(position.pnlCents)}<small>{formatPercentFromBps(position.pnlBps)}</small></em>
                  </button>
                {/each}
              {/if}
            </div>
          </section>

          <section class="panel" aria-label="Order history">
            <div class="panel-head"><div><p class="eyebrow">History</p><h2>Orders</h2></div><Activity size={18} /></div>
            <div class="table">
              <div class="table-head ord"><span>Order</span><span>Ausführung</span><span>Status</span></div>
              {#if (portfolio?.transactions.length ?? 0) === 0}
                <p class="empty-state">Noch keine Trades.</p>
              {:else}
                {#each portfolio?.transactions.slice(0, 18) ?? [] as tx}
                  <div class="table-row ord">
                    <strong class={tx.side}>{tx.side === 'buy' ? 'KAUF' : 'VERKAUF'} {tx.symbol}<small>{formatUpdatedAt(tx.createdAt)}</small></strong>
                    <span>{formatQuantity(tx.quantity)} @ {formatMoney(tx.priceCents)}<small>Gebühr {formatMoney(tx.feeCents)}</small></span>
                    <em class={tx.status === 'synced' ? 'synced-tag' : 'local-tag'}>{tx.status === 'synced' ? 'synced' : 'local'}</em>
                  </div>
                {/each}
              {/if}
            </div>
          </section>
        </div>
      </section>
    {:else if activeView === 'markets'}
      <section class="view-scroll" aria-label="Markets">
        <div class="markets-toolbar panel">
          <label class="search compact" aria-label="Märkte durchsuchen">
            <Search size={16} />
            <input bind:value={marketQuery} type="search" placeholder="Märkte durchsuchen" />
          </label>
          <div class="market-filters wide">
            {#each marketFilters as filter}
              <button class:active={marketFilter === filter.id} type="button" on:click={() => (marketFilter = filter.id)}>{filter.label}</button>
            {/each}
          </div>
        </div>

        <div class="market-grid">
          {#if marketsLoading}
            {#each Array(6) as _}<div class="market-card skeleton"></div>{/each}
          {:else if filteredMarkets.length === 0}
            <p class="empty-state">Keine Märkte für diesen Filter.</p>
          {:else}
            {#each filteredMarkets as item}
              <button class="market-card" class:selected={selectedMarket.assetId === item.assetId} type="button" on:click={() => selectMarket(item.assetId, true)}>
                <div class="card-top">
                  <div><strong>{item.symbol}</strong><small>{item.name}</small></div>
                  <span class="kind-tag">{item.kind}</span>
                </div>
                <Sparkline values={miniSeries(item)} tone={marketTone(item.changeBps)} height={42} />
                <div class="card-bottom">
                  <strong>{formatMoney(item.priceCents)}</strong>
                  <em class={marketTone(item.changeBps)}>{formatPercentFromBps(item.changeBps)}</em>
                </div>
              </button>
            {/each}
          {/if}
        </div>
      </section>
    {:else if activeView === 'esports'}
      <section class="view-scroll" aria-label="eSports">
        <EsportsView
          matches={esportsMatches}
          loading={esportsLoading}
          error={esportsError}
          cashCents={summary.cashCents}
          positions={positionList}
          favoriteTeams={preferences.favoriteTeams}
          selectedLeagues={preferences.esportsLeagues}
          {leagueOptions}
          onBet={placeEsportsBet}
          onSell={sellEsportsPosition}
          onBuyMore={buyMoreEsports}
          onToggleFavorite={toggleFavoriteTeam}
          onToggleLeague={toggleDefaultLeague}
          onRefreshOdds={handleRefreshOdds}
        />
      </section>
    {:else if activeView === 'profile'}
      <section class="view-scroll" aria-label="Profil">
        <ProfileView
          favoriteTeams={preferences.favoriteTeams}
          esportsLeagues={preferences.esportsLeagues}
          teams={esportsTeams}
          {teamsLoading}
          {leagueOptions}
          {clientId}
          equityCents={summary.totalEquityCents}
          startingCents={portfolio?.startingCashCents ?? 0}
          onToggleTeam={toggleFavoriteTeam}
          onToggleLeague={toggleDefaultLeague}
        />
      </section>
    {:else}
      <section class="view-scroll" aria-label="Admin">
        <AdminView
          token={adminToken}
          matches={esportsMatches}
          onLogin={handleAdminLogin}
          onLogout={handleAdminLogout}
        />
      </section>
    {/if}

    {#if marketsError || configError || portfolioError}
      <div class="status-bar">
        {#if configError}<span class="warning">{configError}</span>{/if}
        {#if portfolioError}<span class="warning">{portfolioError}</span>{/if}
        {#if marketsError}<span class="warning">{marketsError}</span>{/if}
        <span class="status-spacer">{syncMessage}</span>
      </div>
    {/if}
  </main>

  {#if showShortcuts}
    <div class="shortcuts-overlay">
      <button class="shortcuts-backdrop" type="button" aria-label="Schließen" on:click={() => (showShortcuts = false)}></button>
      <div class="shortcuts-card" role="dialog" aria-label="Tastenkürzel" aria-modal="true">
        <div class="panel-head"><div><p class="eyebrow">Hilfe</p><h2>Tastenkürzel</h2></div><Keyboard size={18} /></div>
        <ul>
          <li><kbd>B</kbd><span>Buy-Seite</span></li>
          <li><kbd>S</kbd><span>Sell-Seite</span></li>
          <li><kbd>1</kbd>–<kbd>6</kbd><span>Markt wählen</span></li>
          <li><kbd>?</kbd><span>Diese Hilfe</span></li>
        </ul>
        <button class="primary-button" type="button" on:click={() => (showShortcuts = false)}>Schließen</button>
      </div>
    </div>
  {/if}
{/if}
