<script lang="ts">
  import {
    Activity,
    ArrowLeft,
    CandlestickChart,
    CloudUpload,
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
  import InfoTip from './lib/components/InfoTip.svelte';
  import ProfileView from './lib/components/ProfileView.svelte';
  import Toasts from './lib/components/Toasts.svelte';
  import {
    adminLogin,
    changePassword,
    deleteAccount,
    deletePortfolioData,
    exportAccount,
    fetchMe,
    fetchEsportsMatches,
    fetchEsportsResults,
    fetchEsportsTeams,
    fetchMarketHistory,
    fetchMarkets,
    fetchPublicConfig,
    fetchQuotes,
    fetchSyncedPortfolio,
    login,
    logout,
    placeOrder,
    refreshMatchOdds,
    register,
    syncPortfolio,
    updateAccount,
    type Candle,
    type ChartRange,
    type EsportsMatch,
    type EsportsTeam,
    type EsportsTeamInfo,
    type Market,
    type PublicConfig,
    type SessionUser
  } from './lib/api';
  import { loadClientId, loadOpenOrders, loadPortfolio, loadPreferences, resetPortfolio, saveOpenOrders, savePortfolio, savePreferences } from './lib/portfolio-db';
  import { DEFAULT_LEAGUES, MAX_FAVORITE_TEAMS, defaultPreferences, type Preferences } from './lib/preferences';
  import { toast } from './lib/toast';
  import {
    applyTrade,
    computePerformance,
    createInitialPortfolio,
    formatMoney,
    formatPrice,
    formatPercentFromBps,
    markPositionsToMarket,
    resolveEventPosition,
    shouldTriggerOrder,
    summarizePortfolio,
    PORTFOLIO_ID,
    type OpenOrder,
    type OpenOrderType,
    type PortfolioSnapshot
  } from './lib/portfolio';

  const ORDER_FEE_BPS = 8;
  const QUANTITY_STEP = 0.0001;
  const chartRanges: ChartRange[] = ['1H', '1D', '1W', '1M', '1Y'];
  const quantityPresets = [0.25, 0.5, 0.75, 1] as const;

  // Safe placeholder so template/reactive access never dereferences `undefined`
  // when markets fail to load (e.g. backend offline) — avoids a blank screen.
  const EMPTY_MARKET: Market = {
    assetId: '',
    symbol: '—',
    name: 'Keine Marktdaten',
    kind: 'stock',
    source: '',
    priceCents: 0,
    changeBps: 0,
    updatedAt: ''
  };

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
  let markets: Market[] = [];
  let marketsError = '';
  let marketsLoading = true;
  let portfolio: PortfolioSnapshot | null = null;
  let portfolioError = '';
  let selectedAssetId = '';
  let marketQuery = '';
  let marketFilter: MarketFilter = 'all';
  let orderSide: 'buy' | 'sell' = 'buy';
  let orderType: 'market' | OpenOrderType = 'market';
  let orderQuantity: number | string = 1;
  let triggerPrice: number | string = '';
  let orderError = '';
  let openOrders: OpenOrder[] = [];
  let isSyncing = false;
  let syncMessage = 'Sync bereit';
  type AppView = 'landing' | DeskView | 'profile' | 'admin';
  let activeView: AppView = 'landing';
  const ADMIN_TOKEN_KEY = 'koala-admin-token';
  let adminToken: string | null = null;
  let user: SessionUser | null = null;
  let authBusy = false;
  let clientId = '';
  let quoteTimer: ReturnType<typeof setInterval> | undefined;
  let showShortcuts = false;
  let showResetConfirm = false;
  let showOnboardingBanner = false;
  let showTour = false;
  const ONBOARDING_KEY = 'koala-onboarded';

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
    // Non-blocking first-run welcome banner (dismissible, shown on the Trade
    // desk). Set early so it never waits on the slower market/portfolio loads.
    try {
      showOnboardingBanner = localStorage.getItem(ONBOARDING_KEY) !== '1';
    } catch {
      showOnboardingBanner = false;
    }

    try {
      preferences = await loadPreferences();
    } catch {
      preferences = defaultPreferences();
    }

    adminToken = localStorage.getItem(ADMIN_TOKEN_KEY);

    try {
      user = await fetchMe();
    } catch {
      user = null;
    }

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
        selectedAssetId = markets[0]?.assetId ?? '';
      }
    } catch (error) {
      marketsError = error instanceof Error ? error.message : 'Marktdaten nicht verfügbar';
      markets = [];
    } finally {
      marketsLoading = false;
    }

    try {
      portfolio = await loadPortfolio(config?.startingCashCents ?? 1_000_000);
    } catch (error) {
      portfolioError = error instanceof Error ? error.message : 'Lokales Portfolio nicht verfügbar';
      portfolio = createInitialPortfolio(config?.startingCashCents ?? 1_000_000);
    }

    try {
      openOrders = await loadOpenOrders();
    } catch {
      openOrders = [];
    }

    await restoreSyncedPortfolio(!!user);
    await refreshQuotes();
    await loadHistory();
    void settleResolvedBets();
    quoteTimer = setInterval(refreshQuotes, 30_000);
  });

  function dismissOnboarding() {
    showOnboardingBanner = false;
    showTour = false;
    try {
      localStorage.setItem(ONBOARDING_KEY, '1');
    } catch {
      // Best-effort; a private window just sees the welcome again next time.
    }
  }

  onDestroy(() => {
    if (quoteTimer) clearInterval(quoteTimer);
  });

  $: summary = summarizePortfolio(portfolio ?? createInitialPortfolio(config?.startingCashCents ?? 1_000_000));
  $: selectedMarket = markets.find((item) => item.assetId === selectedAssetId) ?? markets[0] ?? EMPTY_MARKET;
  $: query = marketQuery.trim().toLowerCase();
  $: filteredMarkets = markets.filter((market) => {
    const matchesFilter = marketFilter === 'all' || market.kind === marketFilter;
    const matchesQuery = !query || `${market.symbol} ${market.name} ${market.kind}`.toLowerCase().includes(query);
    return matchesFilter && matchesQuery;
  });

  $: effectivePriceCents = selectedMarket.priceCents;
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
  $: orderPowerValue = orderSide === 'buy' ? formatMoney(summary.cashCents) : `${formatQuantity(selectedPositionQuantity)} ${selectedMarket ? selectedMarket.symbol : ''}`;

  // --- Order-type (Market / Limit / Stop) ---------------------------------
  $: isOpenOrderType = orderType !== 'market';
  $: triggerPriceCents = Math.round((Number.isFinite(Number(triggerPrice)) ? Number(triggerPrice) : 0) * 100);
  $: orderTypeHint =
    orderType === 'market'
      ? 'Wird sofort zum aktuellen Marktpreis ausgeführt.'
      : orderType === 'limit'
        ? orderSide === 'buy'
          ? 'Kauf-Limit: füllt erst, wenn der Kurs auf dein Limit oder darunter fällt.'
          : 'Verkaufs-Limit: füllt erst, wenn der Kurs auf dein Limit oder darüber steigt.'
        : orderSide === 'buy'
          ? 'Stop-Buy: löst aus, wenn der Kurs auf deinen Stop oder darüber steigt.'
          : 'Stop-Loss: löst aus, wenn der Kurs auf deinen Stop oder darunter fällt.';
  $: orderStatusLabel = isOpenOrderType ? 'Landet als' : 'Ausführung';
  $: orderStatusValue = isOpenOrderType ? 'Offene Order (wartet)' : 'Sofort';
  $: assetOpenOrders = openOrders.filter((order) => order.assetId === selectedMarket.assetId);
  $: canPlaceOpenOrder =
    !!portfolio &&
    normalizedOrderQuantity > 0 &&
    triggerPriceCents > 0 &&
    (orderSide === 'sell' ? selectedPositionQuantity >= normalizedOrderQuantity : true);
  $: canPlaceOrder = isOpenOrderType ? canPlaceOpenOrder : canSubmitOrder;
  $: submitLabel = isOpenOrderType
    ? `${orderSide === 'buy' ? 'Kauf' : 'Verkauf'}-Order vormerken`
    : `${orderSide === 'buy' ? 'Kaufen' : 'Verkaufen'} ${selectedMarket.symbol}`;
  $: positionList = portfolio?.positions ?? [];
  $: transactionList = portfolio?.transactions.slice(0, 6) ?? [];
  $: positionRows = positionList.map((position) => {
    const marketValueCents = Math.round(position.quantity * position.lastPriceCents);
    const costBasisCents = Math.round(position.quantity * position.averageCostCents);
    const pnlCents = marketValueCents - costBasisCents;
    const pnlBps = costBasisCents > 0 ? Math.round((pnlCents / costBasisCents) * 10_000) : 0;
    return { ...position, marketValueCents, pnlCents, pnlBps };
  });
  $: selectedPositionRow = positionRows.find((row) => row.assetId === selectedMarket.assetId);
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
      // Assign synchronously before awaiting the write so a concurrent flow
      // (e.g. the 30s quote refresh) can't read a stale snapshot and clobber it.
      portfolio = snapshot;
      await savePortfolio(snapshot);
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
    user = await fetchMe();
    localStorage.setItem(ADMIN_TOKEN_KEY, token);
    toast.success('Angemeldet', 'Admin-Bereich entsperrt.');
  }

  function handleAdminLogout() {
    adminToken = null;
    localStorage.removeItem(ADMIN_TOKEN_KEY);
  }

  async function handleUserLogin(username: string, password: string) {
    authBusy = true;
    try {
      const payload = await login(username, password);
      user = payload.user;
      if (payload.token) {
        adminToken = payload.token;
        localStorage.setItem(ADMIN_TOKEN_KEY, payload.token);
      }
      const restored = await restoreSyncedPortfolio(true);
      if (restored !== 'restored') await handleSyncPortfolio();
      toast.success('Eingeloggt', 'Portfolio ist mit deinem Account verbunden.');
    } finally {
      authBusy = false;
    }
  }

  async function handleUserRegister(username: string, password: string) {
    authBusy = true;
    try {
      const payload = await register(username, password);
      user = payload.user;
      await handleSyncPortfolio();
      toast.success('Account erstellt', 'Dein lokales Portfolio wurde übernommen.');
    } finally {
      authBusy = false;
    }
  }

  async function handleUserLogout() {
    authBusy = true;
    try {
      await logout();
      user = null;
      adminToken = null;
      localStorage.removeItem(ADMIN_TOKEN_KEY);
      toast.info('Ausgeloggt', 'Lokales Portfolio bleibt auf diesem Gerät.');
    } finally {
      authBusy = false;
    }
  }

  async function handleUpdateAccount(displayName: string) {
    const next = await updateAccount(displayName);
    user = next;
    toast.success('Profil gespeichert', next.displayName);
  }

  async function handleChangePassword(currentPassword: string, newPassword: string) {
    await changePassword(currentPassword, newPassword);
    toast.success('Passwort geändert');
  }

  async function handleExportAccount() {
    const payload = await exportAccount();
    const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `koalatrade-export-${payload.user.username}-${new Date().toISOString().slice(0, 10)}.json`;
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(url);
    toast.success('Export erstellt', `${payload.portfolios.length} Portfolio-Snapshot${payload.portfolios.length === 1 ? '' : 's'}`);
  }

  async function handleDeletePortfolioData(password: string) {
    await deletePortfolioData(password);
    portfolio = await resetPortfolio(config?.startingCashCents ?? portfolio?.startingCashCents ?? 1_000_000);
    syncMessage = 'Portfolio-Daten gelöscht';
    toast.success('Portfolio-Daten gelöscht', 'Serverdaten entfernt und lokale Kopie zurückgesetzt.');
  }

  async function handleDeleteAccount(password: string) {
    await deleteAccount(password);
    user = null;
    adminToken = null;
    localStorage.removeItem(ADMIN_TOKEN_KEY);
    toast.success('Account gelöscht', 'Serverseitige Account-Daten wurden entfernt.');
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
      portfolio = marked;
      await savePortfolio(marked);
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
      portfolio = next;
      await savePortfolio(next);
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
      portfolio = next;
      await savePortfolio(next);
      toast.success('Cash-out', `${position.symbol} für ${formatMoney(grossCents - feeCents)}`);
    } catch (error) {
      toast.error('Cash-out fehlgeschlagen', error instanceof Error ? error.message : undefined);
    }
  }

  async function handleResetPortfolio() {
    showResetConfirm = false;
    portfolio = await resetPortfolio(config?.startingCashCents ?? portfolio?.startingCashCents ?? 1_000_000);
    openOrders = [];
    await saveOpenOrders([]);
    orderError = '';
    toast.info('Portfolio zurückgesetzt', 'Startkapital wiederhergestellt.');
  }

  function setOrderType(type: 'market' | OpenOrderType) {
    orderType = type;
    orderError = '';
    // Seed the trigger field with the current price as a sensible starting point.
    if (type !== 'market' && !triggerPrice && effectivePriceCents > 0) {
      triggerPrice = (effectivePriceCents / 100).toFixed(2);
    }
  }

  async function placeOpenOrder() {
    if (!portfolio) {
      orderError = 'Portfolio wird noch geladen';
      return;
    }
    if (triggerPriceCents <= 0) {
      orderError = orderType === 'limit' ? 'Gib einen gültigen Limit-Preis ein' : 'Gib einen gültigen Stop-Preis ein';
      return;
    }
    if (!canPlaceOpenOrder) {
      orderError = orderSide === 'sell' ? 'Nicht genug verfügbare Position' : 'Ungültige Menge';
      return;
    }

    const order: OpenOrder = {
      id: crypto.randomUUID(),
      assetId: selectedMarket.assetId,
      symbol: selectedMarket.symbol,
      name: selectedMarket.name,
      kind: selectedMarket.kind,
      side: orderSide,
      orderType: orderType as OpenOrderType,
      quantity: normalizedOrderQuantity,
      triggerPriceCents,
      createdAt: new Date().toISOString()
    };
    openOrders = [order, ...openOrders];
    await saveOpenOrders(openOrders);
    toast.success(
      'Order vorgemerkt',
      `${orderType === 'limit' ? 'Limit' : 'Stop'} ${orderSide === 'buy' ? 'Kauf' : 'Verkauf'} · ${formatQuantity(order.quantity)} ${order.symbol} @ ${formatMoney(triggerPriceCents)}`
    );
    orderError = '';

    // Instant fill guard: if the trigger is already satisfied at the current
    // price, execute now so the queue never holds an already-met order.
    void evaluateOpenOrders(markets);
  }

  async function cancelOpenOrder(id: string) {
    openOrders = openOrders.filter((order) => order.id !== id);
    await saveOpenOrders(openOrders);
  }

  async function handleSubmitOrder() {
    orderError = '';
    if (isOpenOrderType) {
      await placeOpenOrder();
      return;
    }
    if (!portfolio) {
      orderError = 'Portfolio wird noch geladen';
      return;
    }
    if (effectivePriceCents <= 0) {
      orderError = 'Kein gültiger Marktpreis verfügbar';
      return;
    }
    if (!canSubmitOrder) {
      orderError = orderSide === 'buy' ? 'Nicht genug Kaufkraft für diese Order' : 'Nicht genug verfügbare Position';
      return;
    }

    // Competitive mode: when the backend is reachable, market orders are
    // executed and priced server-side (authoritative) so a client can't
    // fabricate the fill. Only when fully offline do we fall back to the local
    // simulation (unranked practice).
    if (config && !configError) {
      await submitServerOrder();
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
      portfolio = nextPortfolio;
      await savePortfolio(nextPortfolio);
      toast.success(
        `${orderSide === 'buy' ? 'Kauf' : 'Verkauf'} ausgeführt`,
        `${formatQuantity(normalizedOrderQuantity)} ${selectedMarket.symbol} @ ${formatMoney(effectivePriceCents)}`
      );
    } catch (error) {
      orderError = error instanceof Error ? error.message : 'Order konnte nicht platziert werden';
      toast.error('Order fehlgeschlagen', orderError);
    }
  }

  // Server-authoritative market order. We first push the current local state
  // (including un-synced eSports positions) so the server operates on the
  // latest snapshot, then place the order; the returned portfolio is truth.
  async function submitServerOrder() {
    if (!portfolio) return;
    const symbol = selectedMarket.symbol;
    try {
      const id = clientId || (await loadClientId());
      try {
        await syncPortfolio(id, portfolio);
      } catch {
        // A sync hiccup shouldn't block the trade; the server still applies to
        // its last-known snapshot. Surfaced only if the order itself fails.
      }
      const updated = await placeOrder(id, {
        portfolioId: PORTFOLIO_ID,
        assetId: selectedMarket.assetId,
        side: orderSide,
        quantity: normalizedOrderQuantity
      });
      portfolio = updated;
      await savePortfolio(updated, { touchUpdatedAt: false });
      toast.success(
        `${orderSide === 'buy' ? 'Kauf' : 'Verkauf'} ausgeführt`,
        `${formatQuantity(normalizedOrderQuantity)} ${symbol}`
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

  function isPristinePortfolio(snapshot: PortfolioSnapshot) {
    return (
      snapshot.positions.length === 0 &&
      snapshot.transactions.length === 0 &&
      snapshot.cashCents === snapshot.startingCashCents
    );
  }

  async function restoreSyncedPortfolio(preferRemoteIfPristine = false): Promise<'restored' | 'missing' | 'kept' | 'unavailable'> {
    if (!portfolio || !clientId || configError) return 'unavailable';
    try {
      const synced = await fetchSyncedPortfolio(clientId, PORTFOLIO_ID);
      if (!synced) {
        syncMessage = 'Sync bereit';
        return 'missing';
      }
      const remoteIsNewer = new Date(synced.updatedAt).getTime() > new Date(portfolio.updatedAt).getTime();
      const shouldRestore = remoteIsNewer || (preferRemoteIfPristine && isPristinePortfolio(portfolio));
      if (shouldRestore) {
        await savePortfolio(synced, { touchUpdatedAt: false });
        portfolio = synced;
        syncMessage = `Wiederhergestellt ${formatUpdatedAt(synced.updatedAt)}`;
        return 'restored';
      }
      syncMessage = 'Lokales Portfolio aktuell';
      return 'kept';
    } catch {
      syncMessage = 'Sync nicht verfügbar';
      return 'unavailable';
    }
  }

  /**
   * Fills any pending Limit/Stop order whose trigger is met at the latest
   * prices. Runs on every quote poll (and right after an order is queued).
   * A triggered order that can't fill (e.g. insufficient cash) is dropped with
   * a warning rather than retried forever.
   */
  async function evaluateOpenOrders(currentMarkets: Market[]) {
    if (openOrders.length === 0 || !portfolio) return;
    const priceByAsset = new Map(currentMarkets.map((market) => [market.assetId, market.priceCents]));
    const remaining: OpenOrder[] = [];
    let working = portfolio;
    let portfolioChanged = false;

    for (const order of openOrders) {
      const price = priceByAsset.get(order.assetId) ?? 0;
      if (!shouldTriggerOrder(order, price)) {
        remaining.push(order);
        continue;
      }
      const grossCents = Math.round(order.quantity * price);
      const feeCents = Math.max(0, Math.round((grossCents * ORDER_FEE_BPS) / 10_000));
      try {
        working = applyTrade(working, {
          id: crypto.randomUUID(),
          assetId: order.assetId,
          symbol: order.symbol,
          name: order.name,
          kind: order.kind,
          side: order.side,
          quantity: order.quantity,
          priceCents: price,
          feeCents
        });
        portfolioChanged = true;
        toast.success(
          'Order ausgeführt',
          `${order.orderType === 'limit' ? 'Limit' : 'Stop'} ${order.side === 'buy' ? 'Kauf' : 'Verkauf'} · ${formatQuantity(order.quantity)} ${order.symbol} @ ${formatMoney(price)}`
        );
      } catch (error) {
        toast.error('Order storniert', `${order.symbol}: ${error instanceof Error ? error.message : 'konnte nicht ausgeführt werden'}`);
      }
    }

    if (portfolioChanged) {
      portfolio = working;
      await savePortfolio(working);
    }
    if (remaining.length !== openOrders.length) {
      openOrders = remaining;
      await saveOpenOrders(remaining);
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

      await evaluateOpenOrders(markets);

      // Background eSports matches updates if relevant
      const hasEsportsPositions = portfolio?.positions.some((p) => p.kind === 'event');
      if (hasEsportsPositions || esportsLoaded) {
        try {
          esportsMatches = await fetchEsportsMatches();
        } catch (e) {
          console.warn('eSports matches update failed in background:', e);
        }
      }

      if (portfolio) {
        let marked = markPositionsToMarket(portfolio, quotes);
        if (esportsMatches.length > 0) {
          const priceByAsset = new Map<string, number>();
          for (const match of esportsMatches) {
            if (!match.hasOdds) continue;
            for (const team of [match.team1, match.team2]) {
              if (team.priceCents > 0) priceByAsset.set(esportsAssetId(match.id, team.code), team.priceCents);
            }
          }
          const updates = marked.positions
            .filter((position) => priceByAsset.has(position.assetId))
            .map((position) => ({ assetId: position.assetId, priceCents: priceByAsset.get(position.assetId)! }));
          if (updates.length > 0) {
            marked = markPositionsToMarket(marked, updates);
          }
        }
        if (marked !== portfolio) {
          portfolio = marked;
          await savePortfolio(marked);
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
    // eSports event markets aren't in the tradable catalogue; route them to the
    // eSports desk instead of silently falling back to an unrelated stock.
    if (assetId.startsWith('event:')) {
      setActiveView('esports');
      return;
    }
    selectedAssetId = assetId;
    orderError = '';
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
      <button class="nav-action" type="button" title="Wechsle direkt zum interaktiven Trading Desk." on:click={() => setActiveView('trade')}>Trading Desk öffnen</button>
    </header>

    <section class="landing-hero" aria-label="KoalaTrade introduction">
      <div class="landing-copy">
        <p class="eyebrow"><Sparkles size={14} /> Virtuelles Trading-Cockpit</p>
        <h1>Märkte meistern, ohne echtes Geld zu riskieren.</h1>
        <p>
          KoalaTrade vereint Aktien, ETFs, Crypto, Rohstoffe und eSports-Eventmärkte in einem schnellen Paper-Trading-Desk —
          zum Lernen und Üben, ganz ohne echtes Risiko.
        </p>
        <div class="landing-actions">
          <button class="primary-button" type="button" title="Starte den KoalaTrade Trading Desk." on:click={() => setActiveView('trade')}>Desk starten</button>
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
          <strong>{formatPrice(selectedMarket.priceCents)}</strong>
          <em class={selectedMarket.priceCents > 0 ? marketTone(selectedMarket.changeBps) : ''}>{selectedMarket.priceCents > 0 ? formatPercentFromBps(selectedMarket.changeBps) : '—'}</em>
        </div>

        <div class="preview-chart">
          <AreaChart
            series={candles.map((c) => c.close)}
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
  <div class="app-shell">
    <nav class="icon-rail" aria-label="Hauptnavigation">
      <div class="rail-logo" aria-hidden="true"></div>
      {#each deskTabs as tab}
        <button class="rail-item" class:active={activeView === tab.id} type="button"
                title={tab.id === 'trade' ? 'Handelsbildschirm: Kaufe und verkaufe Assets zum aktuellen Marktpreis' :
                       tab.id === 'portfolio' ? 'Portfolio: Zeige deine Positionen, deinen Kontostand, P&L-Statistiken und Wertentwicklung' :
                       tab.id === 'markets' ? 'Märkte: Übersicht aller handelbaren Aktien, ETFs, Kryptowährungen und Rohstoffe' :
                       tab.id === 'esports' ? 'eSports: Vorhersagemärkte für anstehende Matches mit Polymarket-Quoten' : ''}
                on:click={() => setActiveView(tab.id)}>
          <svelte:component this={tab.icon} size={17} />
          <span>{tab.label}</span>
        </button>
      {/each}
      <button class="rail-item rail-avatar" class:active={activeView === 'profile'} type="button"
              title="Profil & Favoriten: Verwalte deine Einstellungen, Lieblingsteams und Kontodaten"
              on:click={() => setActiveView('profile')}>
        <UserCircle2 size={18} />
        <span>Profil</span>
      </button>
    </nav>

  <main class="trading-shell">
    <header class="trading-topbar">
      <div class="brand">
        <img src="/icons/koalatrade.svg" alt="" width="34" height="34" />
        <div>
          <strong>KoalaTrade</strong>
          <span>Live paper exchange</span>
        </div>
      </div>

      <div class="desk-actions">
        <span class:online={config && !configError} class="status-pill" title="Status der Verbindung zum KoalaTrade-Backend-Server.">
          <i class="dot"></i>{config && !configError ? 'API online' : 'Local mode'}
        </span>
        <button class="icon-button" class:active={activeView === 'admin'} type="button" aria-label="Admin" title="Admin-Bereich: Teammappings verwalten, Cache leeren und Registrierungsmodus umschalten" on:click={() => setActiveView('admin')}>
          <ShieldCheck size={18} />
        </button>
        <button class="icon-button" type="button" aria-label="Shortcuts" title="Tastenkürzel anzeigen (?)" on:click={() => (showShortcuts = !showShortcuts)}>
          <Keyboard size={18} />
        </button>
        <button class="icon-button" type="button" aria-label="Portfolio synchronisieren" title="Portfolio synchronisieren: Sichere dein Portfolio auf dem Server, um es geräteübergreifend zu nutzen" disabled={isSyncing || !portfolio || !!configError} on:click={handleSyncPortfolio}>
          <CloudUpload size={18} />
        </button>
        <button class="icon-button" type="button" aria-label="Portfolio zurücksetzen" title="Portfolio zurücksetzen: Löscht alle Positionen und setzt dein Guthaben auf den Startwert zurück" on:click={() => (showResetConfirm = true)}>
          <RotateCcw size={18} />
        </button>
      </div>
    </header>

    <section class="market-tape" aria-label="Market tape">
      {#each markets.slice(0, 6) as item, index}
        <button class:active={selectedMarket && selectedMarket.assetId === item.assetId} type="button" title={`Wähle ${item.symbol} aus, um den Chart und das Order-Ticket anzuzeigen.`} on:click={() => selectMarket(item.assetId, true)}>
          <span class="tape-key">{index + 1}</span>
          <strong>{item.symbol}</strong>
          <span class="tape-price">{formatPrice(item.priceCents)}</span>
          <em class={item.priceCents > 0 ? marketTone(item.changeBps) : ''}>{item.priceCents > 0 ? formatPercentFromBps(item.changeBps) : '—'}</em>
        </button>
      {/each}
    </section>

    {#if activeView === 'trade'}
      {#if showOnboardingBanner}
        <div class="onboarding-banner" role="note">
          <div class="ob-text">
            <span class="ob-emoji" aria-hidden="true">👋</span>
            <div>
              <strong>Neu hier? Du handelst mit {formatMoney(config?.startingCashCents ?? 1_000_000)} Spielgeld.</strong>
              <p>Wähle links einen Markt, rechts einen Order-Typ – und platziere deinen ersten Trade.</p>
            </div>
          </div>
          <div class="ob-actions">
            <button type="button" class="ob-tour" title="Kurze Einführung in KoalaTrade ansehen" on:click={() => (showTour = true)}>Tour ansehen</button>
            <button type="button" class="ob-dismiss" title="Diesen Hinweis dauerhaft ausblenden" on:click={dismissOnboarding}>Ausblenden</button>
          </div>
        </div>
      {/if}
      <section class="trade-layout" aria-label="Trading workspace">
        <aside class="watchlist panel" aria-label="Markets">
          <label class="search compact" aria-label="Märkte durchsuchen">
            <Search size={16} />
            <input bind:value={marketQuery} type="search" placeholder="Märkte durchsuchen" />
          </label>
          <div class="market-filters" aria-label="Markt-Filter">
            {#each marketFilters as filter}
              <button class:active={marketFilter === filter.id} type="button" title={`Zeige nur Märkte vom Typ "${filter.label}"`} on:click={() => (marketFilter = filter.id)}>{filter.label}</button>
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
                <button class:selected={selectedMarket && selectedMarket.assetId === item.assetId} class="market-row" type="button" title={`Wähle ${item.symbol} (${item.name}) aus.`} on:click={() => selectMarket(item.assetId)}>
                  <span class="asset"><strong>{item.symbol}</strong><small>{item.kind}</small></span>
                  {#if item.priceCents > 0}
                    <span class="price">{formatMoney(item.priceCents)}</span>
                    <em class={marketTone(item.changeBps)}>{formatPercentFromBps(item.changeBps)}</em>
                  {:else}
                    <span class="no-feed" title="Für dieses Asset liegt aktuell kein Live-Kurs vor (kein Feed/API-Key)."><i></i>no feed</span>
                  {/if}
                </button>
              {/each}
            {/if}
          </div>
        </aside>

        <section class="market-stage">
          <section class="instrument-strip panel" aria-label="Selected market">
            {#if selectedMarket}
              <div class="instrument-id">
                <p class="eyebrow">{selectedMarket.kind} · {selectedMarket.source}</p>
                <h1>{selectedMarket.symbol}</h1>
                <span>{selectedMarket.name}</span>
              </div>
              <div class="instrument-price">
                <strong>{formatPrice(selectedMarket.priceCents)}</strong>
                <span class={selectedMarket.priceCents > 0 ? marketTone(selectedMarket.changeBps) : ''}>
                  {selectedMarket.priceCents > 0 ? formatPercentFromBps(selectedMarket.changeBps) + ' heute' : '—'}
                </span>
              </div>
            {:else}
              <div class="instrument-id">
                <p class="eyebrow">—</p>
                <h1>Kein Markt ausgewählt</h1>
              </div>
            {/if}
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
                {#if selectedMarket && selectedMarket.priceCents > 0}
                  <h2>{formatMoney(selectedMarket.priceCents)} <em class={changeColor(rangeChangeBps)}>{formatSignedMoney(rangeChangeCents)} ({formatPercentFromBps(rangeChangeBps)})</em></h2>
                {:else}
                  <h2>—</h2>
                {/if}
              </div>
              <div class="chart-controls">
                <button class="sma-toggle" class:active={showSma} type="button" title="Simple Moving Average (14): Blendet den gleitenden Durchschnitt der letzten 14 Kerzen ein/aus, um den Trend zu visualisieren." on:click={() => (showSma = !showSma)}>SMA 14</button>
                <InfoTip placement="bottom" text="Simple Moving Average (14): der gleitende Durchschnitt der letzten 14 Kerzen. Glättet den Kurs und zeigt den Trend – liegt der Preis darüber, ist der kurzfristige Trend eher aufwärts." />

                <div class="timeframes">
                  {#each chartRanges as range}
                    <button class:active={chartRange === range} type="button" title={`Ändere den Chart-Zeitraum auf ${range}`} on:click={() => (chartRange = range)}>{range}</button>
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
              <span>Spanne<InfoTip placement="bottom" text="Die prozentuale Differenz zwischen dem höchsten und tiefsten Kurs im gewählten Zeitraum – ein Maß für die Schwankung (Volatilität)." /> <strong>{formatPercentFromBps(chartLow > 0 ? Math.round(((chartHigh - chartLow) / chartLow) * 10_000) : 0)}</strong></span>
            </div>
          </section>
        </section>

        <aside class="execution-column" aria-label="Execution">
          <section class="panel market-detail" aria-label="Marktdetails">
            <div class="panel-head"><div><p class="eyebrow">Live · {selectedMarket.source || '—'}</p><h2>Marktdetails</h2></div><Activity size={18} /></div>
            <div class="detail-grid">
              <div><span>Preis</span><strong>{formatPrice(selectedMarket.priceCents)}</strong></div>
              <div><span>24h</span><strong class={selectedMarket.priceCents > 0 ? changeColor(selectedMarket.changeBps) : ''}>{selectedMarket.priceCents > 0 ? formatPercentFromBps(selectedMarket.changeBps) : '—'}</strong></div>
              <div><span>Typ</span><strong>{selectedMarket.kind}</strong></div>
              <div><span>Aktualisiert</span><strong>{selectedMarket.updatedAt ? formatUpdatedAt(selectedMarket.updatedAt) : '—'}</strong></div>
            </div>
            {#if selectedPositionRow}
              <div class="detail-position">
                <p class="eyebrow">Deine Position</p>
                <div class="detail-grid">
                  <div><span>Menge</span><strong>{formatQuantity(selectedPositionRow.quantity)}</strong></div>
                  <div><span>Ø-Einstand<InfoTip text="Dein durchschnittlicher Kaufpreis für diese Position über alle Käufe hinweg." /></span><strong>{formatMoney(selectedPositionRow.averageCostCents)}</strong></div>
                  <div><span>Marktwert</span><strong>{formatMoney(selectedPositionRow.marketValueCents)}</strong></div>
                  <div><span>P&amp;L<InfoTip text="Profit &amp; Loss: der aktuelle Gewinn oder Verlust dieser Position – Marktwert minus Einstandswert." /></span><strong class={changeColor(selectedPositionRow.pnlCents)}>{formatSignedMoney(selectedPositionRow.pnlCents)}</strong></div>
                </div>
              </div>
            {:else}
              <p class="panel-note">Noch keine Position in {selectedMarket.symbol}.</p>
            {/if}
          </section>

          <section class="order-panel panel" aria-label="Order ticket">
            <div class="panel-head">
              <div><p class="eyebrow">Order-Ticket · {orderType === 'market' ? 'Market' : orderType === 'limit' ? 'Limit' : 'Stop'}</p><h2>{orderSide === 'buy' ? 'Kaufen' : 'Verkaufen'} {selectedMarket.symbol}</h2></div>
              <Zap size={18} />
            </div>
            <form class="order-form" on:submit|preventDefault={handleSubmitOrder}>
              <div class="segmented" aria-label="Order-Seite">
                <button class:active={orderSide === 'buy'} type="button" title="Kauf-Order: Assets erwerben" on:click={() => setOrderSide('buy')}>Kaufen</button>
                <button class:active={orderSide === 'sell'} class="sell" type="button" title="Verkaufs-Order: Assets aus deinem Bestand veräußern" on:click={() => setOrderSide('sell')}>Verkaufen</button>
              </div>

              <div class="order-types" role="tablist" aria-label="Order-Typ">
                <button class:active={orderType === 'market'} type="button" role="tab" aria-selected={orderType === 'market'} title="Market-Order: wird sofort zum aktuellen Marktpreis ausgeführt." on:click={() => setOrderType('market')}>Market</button>
                <button class:active={orderType === 'limit'} type="button" role="tab" aria-selected={orderType === 'limit'} title="Limit-Order: wird erst ausgeführt, wenn der Kurs dein Limit erreicht – bleibt bis dahin als offene Order stehen." on:click={() => setOrderType('limit')}>Limit</button>
                <button class:active={orderType === 'stop'} type="button" role="tab" aria-selected={orderType === 'stop'} title="Stop-Order: löst bei Erreichen deines Stop-Preises aus und wird dann zur Marktorder – bleibt bis dahin offen." on:click={() => setOrderType('stop')}>Stop</button>
              </div>

              <p class="order-hint">{orderTypeHint}</p>

              <label class="field" title="Menge: Gib die Stückzahl ein, die du handeln möchtest.">
                <span>Menge</span>
                <input bind:value={orderQuantity} min="0.0001" step="0.0001" type="number" title="Gewünschte Stückzahl für die Order" />
              </label>

              {#if orderType === 'limit'}
                <label class="field trigger limit" title="Limit-Preis: Kauf füllt nur zu diesem Preis oder besser (tiefer), Verkauf nur zu diesem oder besser (höher).">
                  <span>Limit-Preis <small>nur zu diesem Preis oder besser</small></span>
                  <input bind:value={triggerPrice} min="0" step="0.01" type="number" placeholder={(effectivePriceCents / 100).toFixed(2)} title="Kurs, bei dem die Limit-Order füllt" />
                </label>
              {:else if orderType === 'stop'}
                <label class="field trigger stop" title="Stop-Preis: Sobald der Kurs diesen Wert erreicht, wird die Order als Marktorder ausgeführt.">
                  <span>Stop-Preis <small>löst als Marktorder aus</small></span>
                  <input bind:value={triggerPrice} min="0" step="0.01" type="number" placeholder={(effectivePriceCents / 100).toFixed(2)} title="Auslöse-Kurs der Stop-Order" />
                </label>
              {/if}

              {#if orderType === 'market'}
                <div class="presets" aria-label="Mengen-Presets">
                  {#each quantityPresets as preset}
                    <button type="button" disabled={orderLimitQuantity <= 0} title={`Setzt die Menge auf ${Math.round(preset * 100)}% deines verfügbaren Budgets bzw. Bestands`} on:click={() => applyPreset(preset)}>{Math.round(preset * 100)}%</button>
                  {/each}
                </div>
              {/if}

              <div class="order-power"><span>{orderPowerLabel}</span><strong>{orderPowerValue}</strong></div>

              <div class="order-summary">
                {#if isOpenOrderType}
                  <div title="Kurs, bei dem diese Order auslöst."><span>{orderType === 'limit' ? 'Limit-Preis' : 'Stop-Preis'}</span><strong>{triggerPriceCents > 0 ? formatMoney(triggerPriceCents) : '—'}</strong></div>
                  <div title="Ordervolumen zum Trigger-Preis (ohne Gebühr)."><span>Volumen</span><strong>{triggerPriceCents > 0 ? formatMoney(Math.round(normalizedOrderQuantity * triggerPriceCents)) : '—'}</strong></div>
                {:else}
                  <div title="Der aktuelle Preis pro Einheit des Assets."><span>Marktpreis</span><strong>{formatMoney(effectivePriceCents)}</strong></div>
                  <div title="Reiner Preis der Einheiten ohne Gebühren."><span>Bruttowert</span><strong>{formatMoney(estimatedOrderValue)}</strong></div>
                  <div title="Simulierte Transaktionsgebühr für diesen Trade."><span>Gebühr ({(ORDER_FEE_BPS / 100).toFixed(2)}%)<InfoTip text={`Simulierte Handelsgebühr von ${(ORDER_FEE_BPS / 100).toFixed(2)}% auf den Ordervolumen – wie bei einem echten Broker, damit die Simulation realistisch bleibt.`} /></span><strong>{formatMoney(estimatedOrderFee)}</strong></div>
                  <div class="total" title="Gesamter Cash-Betrag, der nach Gebühren belastet oder gutgeschrieben wird."><span>{orderSide === 'buy' ? 'Cash-Belastung' : 'Cash-Gutschrift'}</span><strong>{formatMoney(estimatedOrderTotal)}</strong></div>
                {/if}
                <div class="status-row" title="Ob die Order sofort füllt oder als offene Order auf ihren Trigger wartet."><span>{orderStatusLabel}</span><strong>{orderStatusValue}</strong></div>
              </div>

              {#if orderError}<p class="form-error">{orderError}</p>{/if}
              <button class="primary-button" class:danger={orderSide === 'sell'} type="submit" title={isOpenOrderType ? 'Order in die Warteschlange offener Orders legen' : orderSide === 'buy' ? `Simulierten Kauf von ${normalizedOrderQuantity}x ${selectedMarket.symbol} ausführen` : `Simulierten Verkauf von ${normalizedOrderQuantity}x ${selectedMarket.symbol} ausführen`} disabled={!canPlaceOrder}>
                {submitLabel}
              </button>
            </form>
          </section>

          {#if openOrders.length > 0}
            <section class="panel open-orders" aria-label="Offene Orders">
              <div class="panel-head"><div><p class="eyebrow">Warteschlange</p><h2>Offene Orders ({openOrders.length})</h2></div><Activity size={18} /></div>
              <div class="open-orders-list">
                {#each openOrders as order (order.id)}
                  <div class="open-order-row" class:this-asset={order.assetId === selectedMarket.assetId}>
                    <div class="oo-id">
                      <strong>{order.orderType === 'limit' ? 'Limit' : 'Stop'} {order.side === 'buy' ? 'Kauf' : 'Verkauf'}</strong>
                      <small>{formatQuantity(order.quantity)} {order.symbol} @ {formatMoney(order.triggerPriceCents)}</small>
                    </div>
                    <span class="oo-status">wartet</span>
                    <button type="button" class="oo-cancel" title="Diese offene Order stornieren" on:click={() => cancelOpenOrder(order.id)}>Stornieren</button>
                  </div>
                {/each}
              </div>
            </section>
          {/if}
        </aside>
      </section>
    {:else if activeView === 'portfolio'}
      <section class="view-scroll" aria-label="Portfolio">
        <section class="portfolio-metrics">
          <div class="metric primary">
            <span>Equity<InfoTip placement="bottom" text="Dein gesamter Portfoliowert: verfügbares Cash plus der aktuelle Marktwert aller offenen Positionen." /></span>
            <strong>{formatMoney(summary.totalEquityCents)}</strong>
            <em class={changeColor(summary.totalReturnBps)}>{formatSignedMoney(summary.totalReturnCents)} ({formatPercentFromBps(summary.totalReturnBps)})</em>
          </div>
          <div class="metric"><span>Cash</span><strong>{formatMoney(summary.cashCents)}</strong><em>{summary.openPositions} Positionen</em></div>
          <div class="metric"><span>Realisierter P&L<InfoTip placement="bottom" text="Der bereits festgestellte Gewinn/Verlust aus verkauften (geschlossenen) Positionen – Geld, das du tatsächlich realisiert hast." /></span><strong class={changeColor(performance.realizedPnlCents)}>{formatSignedMoney(performance.realizedPnlCents)}</strong><em>geschlossen</em></div>
          <div class="metric"><span>Unrealisiert<InfoTip placement="bottom" text="Der Buchgewinn/-verlust deiner noch offenen Positionen zum aktuellen Kurs – noch nicht realisiert, ändert sich mit dem Preis." /></span><strong class={changeColor(performance.unrealizedPnlCents)}>{formatSignedMoney(performance.unrealizedPnlCents)}</strong><em>offen</em></div>
          <div class="metric"><span>Max Drawdown<InfoTip placement="bottom" align="right" text="Der größte prozentuale Rückgang deiner Equity vom bisherigen Höchststand (Peak) bis zum Tief – ein Risikomaß dafür, wie tief es zwischenzeitlich runterging." /></span><strong class:down={performance.drawdownBps > 0}>{formatPercentFromBps(-performance.drawdownBps)}</strong><em>Peak {formatMoney(performance.peakEquityCents)}</em></div>
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
                <button class:active={positionSort === 'value'} type="button" title="Sortiere deine offenen Positionen nach Marktwert" on:click={() => (positionSort = 'value')}>Wert</button>
                <button class:active={positionSort === 'pnl'} type="button" title="Sortiere deine offenen Positionen nach Gewinn/Verlust" on:click={() => (positionSort = 'pnl')}>P&L</button>
              </div>
            </div>
            <div class="table">
              <div class="table-head pos"><span>Asset</span><span>Menge</span><span>Wert</span><span>P&L</span></div>
              {#if sortedPositionRows.length === 0}
                <p class="empty-state">Noch keine offenen Positionen.</p>
              {:else}
                {#each sortedPositionRows as position}
                  <button class="table-row pos" type="button" title={`Klicke hier, um den Trading-Desk für ${position.symbol} zu öffnen und die Position zu handeln.`} on:click={() => selectMarket(position.assetId, true)}>
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
              <button class:active={marketFilter === filter.id} type="button" title={`Zeige nur Märkte vom Typ "${filter.label}"`} on:click={() => (marketFilter = filter.id)}>{filter.label}</button>
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
              <button class="market-card" class:selected={selectedMarket.assetId === item.assetId} type="button" title={`Öffne den Trading-Desk für ${item.symbol} (${item.name}).`} on:click={() => selectMarket(item.assetId, true)}>
                <div class="card-top">
                  <div><strong>{item.symbol}</strong><small>{item.name}</small></div>
                  <span class="kind-tag">{item.kind}</span>
                </div>
                <div class="card-bottom">
                  <strong>{formatPrice(item.priceCents)}</strong>
                  <em class={item.priceCents > 0 ? marketTone(item.changeBps) : ''}>{item.priceCents > 0 ? formatPercentFromBps(item.changeBps) : '—'}</em>
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
          {user}
          registrationOpen={config?.registrationOpen ?? true}
          {authBusy}
          equityCents={summary.totalEquityCents}
          startingCents={portfolio?.startingCashCents ?? 0}
          onToggleTeam={toggleFavoriteTeam}
          onToggleLeague={toggleDefaultLeague}
          onLogin={handleUserLogin}
          onRegister={handleUserRegister}
          onLogout={handleUserLogout}
          onUpdateAccount={handleUpdateAccount}
          onChangePassword={handleChangePassword}
          onExportAccount={handleExportAccount}
          onDeletePortfolioData={handleDeletePortfolioData}
          onDeleteAccount={handleDeleteAccount}
        />
      </section>
    {:else}
      <section class="view-scroll" aria-label="Admin">
        <AdminView
          token={adminToken}
          matches={esportsMatches}
          onLogin={handleAdminLogin}
          onLogout={handleAdminLogout}
          onRefreshMatches={loadEsports}
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
  </div>

  {#if showShortcuts}
    <div class="shortcuts-overlay">
      <button class="shortcuts-backdrop" type="button" aria-label="Schließen" title="Hilfefenster schließen" on:click={() => (showShortcuts = false)}></button>
      <div class="shortcuts-card" role="dialog" aria-label="Tastenkürzel" aria-modal="true">
        <div class="panel-head"><div><p class="eyebrow">Hilfe</p><h2>Tastenkürzel</h2></div><Keyboard size={18} /></div>
        <ul>
          <li><kbd>B</kbd><span>Buy-Seite</span></li>
          <li><kbd>S</kbd><span>Sell-Seite</span></li>
          <li><kbd>1</kbd>–<kbd>6</kbd><span>Markt wählen</span></li>
          <li><kbd>?</kbd><span>Diese Hilfe</span></li>
        </ul>
        <button class="primary-button" type="button" title="Tastenkürzel-Fenster schließen" on:click={() => (showShortcuts = false)}>Schließen</button>
      </div>
    </div>
  {/if}

  {#if showResetConfirm}
    <div class="shortcuts-overlay">
      <button class="shortcuts-backdrop" type="button" aria-label="Schließen" title="Dialog schließen und abbrechen" on:click={() => (showResetConfirm = false)}></button>
      <div class="shortcuts-card" role="dialog" aria-label="Portfolio zurücksetzen" aria-modal="true">
        <div class="panel-head"><div><p class="eyebrow">Bestätigen</p><h2>Portfolio zurücksetzen?</h2></div><RotateCcw size={18} /></div>
        <p class="confirm-text">Alle Positionen, Trades und dein Verlauf werden gelöscht und das Startkapital von {formatMoney(portfolio?.startingCashCents ?? config?.startingCashCents ?? 1_000_000)} wiederhergestellt. Das kann nicht rückgängig gemacht werden.</p>
        <div class="confirm-actions">
          <button class="ghost-button" type="button" title="Zurücksetzen abbrechen" on:click={() => (showResetConfirm = false)}>Abbrechen</button>
          <button class="primary-button danger" type="button" title="Alle Trades löschen und Guthaben auf Startkapital zurücksetzen" on:click={handleResetPortfolio}>Zurücksetzen</button>
        </div>
      </div>
    </div>
  {/if}

  {#if showTour}
    <div class="shortcuts-overlay">
      <button class="shortcuts-backdrop" type="button" aria-label="Schließen" title="Tour schließen" on:click={() => (showTour = false)}></button>
      <div class="shortcuts-card onboarding-card" role="dialog" aria-label="Willkommen" aria-modal="true">
        <div class="panel-head"><div><p class="eyebrow">Willkommen bei KoalaTrade</p><h2>Paper-Trading in 30 Sekunden</h2></div><Sparkles size={18} /></div>
        <ul class="onboarding-list">
          <li><WalletCards size={16} /><span>Du startest mit <strong>{formatMoney(config?.startingCashCents ?? 1_000_000)}</strong> Spielgeld – <strong>kein echtes Risiko</strong>.</span></li>
          <li><CandlestickChart size={16} /><span>Handle Aktien, ETFs, Krypto und Rohstoffe zum Live-Marktpreis über das Order-Ticket.</span></li>
          <li><Trophy size={16} /><span>Wette im <strong>eSports</strong>-Bereich auf LoL-Matches – Yes zahlt {formatMoney(10_000)} bei Sieg.</span></li>
          <li><UserCircle2 size={16} /><span>Optional: Account anlegen, um dein Portfolio geräteübergreifend zu synchronisieren.</span></li>
        </ul>
        <button class="primary-button" type="button" title="Paper-Trading starten" on:click={dismissOnboarding}>Los geht's</button>
      </div>
    </div>
  {/if}
{/if}
