<script lang="ts">
  import {
    Activity,
    ArrowLeft,
    Award,
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
  import { get } from 'svelte/store';
  import AdminView from './lib/components/AdminView.svelte';
  import AreaChart from './lib/components/AreaChart.svelte';
  import EsportsView from './lib/components/EsportsView.svelte';
  import InfoTip from './lib/components/InfoTip.svelte';
  import LeaderboardView from './lib/components/LeaderboardView.svelte';
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
    fetchLeaderboard,
    fetchMarketHistory,
    fetchMarkets,
    fetchPublicConfig,
    cancelServerOpenOrder,
    fetchOpenOrders,
    fetchQuotes,
    fetchSyncedPortfolio,
    login,
    logout,
    placeOrder,
    refreshMatchOdds,
    register,
    submitEsportsBet,
    syncPortfolio,
    updateAccount,
    type Candle,
    type ChartRange,
    type EsportsMatch,
    type EsportsTeam,
    type EsportsTeamInfo,
    type LeaderboardEntry,
    type Market,
    type PublicConfig,
    type SessionUser
  } from './lib/api';
  import { priceFreshness, isStalePrice, marketTone, changeColor, simpleMovingAverage } from './lib/market-utils';
  import { loadClientId, loadOpenOrders, loadPortfolio, loadPreferences, resetPortfolio, saveOpenOrders, savePortfolio, savePreferences } from './lib/portfolio-db';
  import { DEFAULT_LEAGUES, MAX_FAVORITE_TEAMS, defaultPreferences, type Preferences } from './lib/preferences';
  import { toast } from './lib/toast';
  import { t, locale, setLocale, LOCALES, LOCALE_LABELS } from './lib/i18n';
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

  // Imperative translator for non-reactive script contexts (toasts, status
  // strings). Resolves against the current locale at call time.
  const tr = (key: string, vars?: Record<string, string | number>) => get(t)(key, vars);

  const ORDER_FEE_BPS = 8;
  const QUANTITY_STEP = 0.0001;
  const chartRanges: ChartRange[] = ['1H', '1D', '1W', '1M', '1Y'];
  const quantityPresets = [0.25, 0.5, 0.75, 1] as const;

  // Safe placeholder so template/reactive access never dereferences `undefined`
  // when markets fail to load (e.g. backend offline) — avoids a blank screen.
  const EMPTY_MARKET: Market = {
    assetId: '',
    symbol: '—',
    name: '',
    kind: 'stock',
    source: '',
    priceCents: 0,
    changeBps: 0,
    updatedAt: ''
  };

  const marketFilters = [
    { id: 'all', labelKey: 'filter.all' },
    { id: 'crypto', labelKey: 'filter.crypto' },
    { id: 'etf', labelKey: 'filter.etf' },
    { id: 'commodity', labelKey: 'filter.commodity' },
    { id: 'event', labelKey: 'filter.event' }
  ] as const;
  type MarketFilter = (typeof marketFilters)[number]['id'];

  const deskTabs = [
    { id: 'trade', labelKey: 'nav.trade', icon: CandlestickChart },
    { id: 'portfolio', labelKey: 'nav.portfolio', icon: WalletCards },
    { id: 'markets', labelKey: 'nav.markets', icon: LineChart },
    { id: 'esports', labelKey: 'nav.esports', icon: Trophy },
    { id: 'leaderboard', labelKey: 'nav.leaderboard', icon: Award }
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
  let syncMessage = tr('sync.ready');
  type AppView = 'landing' | DeskView | 'profile' | 'admin';
  let activeView: AppView = 'landing';
  const viewPaths: Record<AppView, string> = {
    landing: '/',
    trade: '/trade',
    portfolio: '/portfolio',
    markets: '/markets',
    esports: '/esports',
    leaderboard: '/leaderboard',
    profile: '/profile',
    admin: '/admin'
  };
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

  // Leaderboard state
  let leaderboard: LeaderboardEntry[] = [];
  let leaderboardLoading = false;
  let leaderboardLoaded = false;
  let leaderboardError = '';

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

  function viewFromPath(pathname: string): AppView {
    const path = pathname.replace(/\/+$/, '') || '/';
    const match = (Object.entries(viewPaths) as [AppView, string][]).find(([, route]) => route === path);
    return match?.[0] ?? 'landing';
  }

  onMount(async () => {
    activeView = viewFromPath(window.location.pathname);
    if (activeView === 'landing' && window.location.pathname !== '/') {
      window.history.replaceState({}, '', '/');
    }
    window.addEventListener('popstate', handlePopState);

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
      toast.error(tr('toast.syncUnavailable'), error instanceof Error ? error.message : undefined);
    }

    try {
      config = await fetchPublicConfig();
    } catch (error) {
      configError = error instanceof Error ? error.message : tr('errors.backendUnreachable');
    }

    try {
      markets = await fetchMarkets();
      if (!markets.some((market) => market.assetId === selectedAssetId)) {
        selectedAssetId = markets[0]?.assetId ?? '';
      }
    } catch (error) {
      marketsError = error instanceof Error ? error.message : tr('errors.marketDataUnavailable');
      markets = [];
    } finally {
      marketsLoading = false;
    }

    try {
      portfolio = await loadPortfolio(config?.startingCashCents ?? 1_000_000);
    } catch (error) {
      portfolioError = error instanceof Error ? error.message : tr('errors.localPortfolioUnavailable');
      portfolio = createInitialPortfolio(config?.startingCashCents ?? 1_000_000);
    }

    // Open orders: server-managed when online (survive a closed browser),
    // local-only in offline practice.
    try {
      if (config && !configError) {
        openOrders = await fetchOpenOrders(clientId || (await loadClientId()), PORTFOLIO_ID);
        await saveOpenOrders(openOrders);
      } else {
        openOrders = await loadOpenOrders();
      }
    } catch {
      try {
        openOrders = await loadOpenOrders();
      } catch {
        openOrders = [];
      }
    }

    await restoreSyncedPortfolio(!!user);
    await refreshQuotes();
    await loadHistory();
    void settleResolvedBets();
    quoteTimer = setInterval(refreshQuotes, 30_000);

    const onVisibility = () => {
      if (document.hidden) {
        clearInterval(quoteTimer);
        quoteTimer = undefined;
      } else if (!quoteTimer) {
        void refreshQuotes();
        quoteTimer = setInterval(refreshQuotes, 30_000);
      }
    };
    document.addEventListener('visibilitychange', onVisibility);

    // Stash for cleanup in onDestroy (avoid window type pollution).
    const CLEANUP_KEY = '__koala_vis_cleanup';
    (window as any)[CLEANUP_KEY] = () => document.removeEventListener('visibilitychange', onVisibility);
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
    const cleanup = (window as any).__koala_vis_cleanup;
    if (typeof cleanup === 'function') cleanup();
    window.removeEventListener('popstate', handlePopState);
  });

  // Online = backend reachable → trades and open orders are server-authoritative
  // (competitive). Offline falls back to the local simulation (unranked practice).
  $: online = !!config && !configError;
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
  $: orderPowerLabel = orderSide === 'buy' ? $t('order.powerBuy') : $t('order.powerSell');
  $: orderPowerValue = orderSide === 'buy' ? formatMoney(summary.cashCents) : `${formatQuantity(selectedPositionQuantity)} ${selectedMarket ? selectedMarket.symbol : ''}`;

  // --- Order-type (Market / Limit / Stop) ---------------------------------
  $: isOpenOrderType = orderType !== 'market';
  $: triggerPriceCents = Math.round((Number.isFinite(Number(triggerPrice)) ? Number(triggerPrice) : 0) * 100);
  $: orderTypeHint =
    orderType === 'market'
      ? $t('order.hintMarket')
      : orderType === 'limit'
        ? orderSide === 'buy'
          ? $t('order.hintBuyLimit')
          : $t('order.hintSellLimit')
        : orderSide === 'buy'
          ? $t('order.hintStopBuy')
          : $t('order.hintStopLoss');
  $: orderStatusLabel = isOpenOrderType ? $t('order.landsAs') : $t('order.execution');
  $: orderStatusValue = isOpenOrderType ? $t('order.openWaiting') : $t('order.immediate');
  $: assetOpenOrders = openOrders.filter((order) => order.assetId === selectedMarket.assetId);
  $: canPlaceOpenOrder =
    !!portfolio &&
    normalizedOrderQuantity > 0 &&
    triggerPriceCents > 0 &&
    (orderSide === 'sell' ? selectedPositionQuantity >= normalizedOrderQuantity : true);
  $: canPlaceOrder = isOpenOrderType ? canPlaceOpenOrder : canSubmitOrder;
  $: submitLabel = isOpenOrderType
    ? $t('order.queueLabel', { side: orderSide === 'buy' ? $t('side.buyNoun') : $t('side.sellNoun') })
    : $t('order.submitLabel', { side: orderSide === 'buy' ? $t('side.buyVerb') : $t('side.sellVerb'), symbol: selectedMarket.symbol });
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
  // Recomputes whenever the quote refresh reassigns selectedMarket (every 30 s).
  $: selectedMarketFreshness = priceFreshness(selectedMarket);

  // React to chart input changes once data is loaded.
  $: if (activeView !== 'landing' && selectedAssetId && chartRange) {
    void loadHistory();
  }

  // Lazy-load esports matches the first time the tab is opened.
  $: if (activeView === 'esports' && !esportsLoaded && !esportsLoading) {
    void loadEsports();
  }

  // Load the leaderboard when its tab is opened.
  $: if (activeView === 'leaderboard' && !leaderboardLoaded && !leaderboardLoading) {
    void loadLeaderboard();
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

  async function loadLeaderboard() {
    leaderboardLoading = true;
    leaderboardError = '';
    try {
      leaderboard = await fetchLeaderboard();
      leaderboardLoaded = true;
    } catch (error) {
      leaderboardError = error instanceof Error ? error.message : tr('errors.leaderboardUnreachable');
    } finally {
      leaderboardLoading = false;
    }
  }

  async function loadEsports() {
    esportsLoading = true;
    esportsError = '';
    try {
      const nextMatches = await fetchEsportsMatches();
      try {
        esportsTeams = await fetchEsportsTeams();
        teamsLoaded = true;
      } catch {
        // Match loading remains useful when the catalogue endpoint is briefly
        // unavailable; use the last known team image map in that case.
      }
      esportsMatches = withLocalTeamImages(nextMatches);
      esportsLoaded = true;
      await reconcileEsportsPositions();
      await settleResolvedBets();
    } catch (error) {
      esportsError = error instanceof Error ? error.message : tr('errors.esportsFeedUnreachable');
    } finally {
      esportsLoading = false;
    }
  }

  function localTeamImage(image: string) {
    return image.startsWith('/api/esports/teams/') ? image : '';
  }

  function withLocalTeamImages(matches: EsportsMatch[]) {
    const imageByCode = new Map<string, string>();
    for (const team of esportsTeams) {
      const image = localTeamImage(team.image);
      if (image) imageByCode.set(team.code.toUpperCase(), image);
    }
    for (const match of esportsMatches) {
      for (const team of [match.team1, match.team2]) {
        const image = localTeamImage(team.image);
        if (image && !imageByCode.has(team.code.toUpperCase())) imageByCode.set(team.code.toUpperCase(), image);
      }
    }

    return matches.map((match) => ({
      ...match,
      team1: { ...match.team1, image: localTeamImage(match.team1.image) || imageByCode.get(match.team1.code.toUpperCase()) || '' },
      team2: { ...match.team2, image: localTeamImage(match.team2.image) || imageByCode.get(match.team2.code.toUpperCase()) || '' }
    }));
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
    // Online, the server settler owns payouts; running it here too would fight
    // the authoritative snapshot. This path is offline practice only.
    if (online) return;
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
          toast.success(tr('toast.betWon'), tr('toast.betWonDetail', { symbol: item.symbol, amount: formatMoney(item.quantity * 100) }));
        } else {
          toast.info(tr('toast.betLost'), tr('toast.betLostDetail', { symbol: item.symbol }));
        }
      }
    }
  }

  async function buyMoreEsports(assetId: string, contracts: number) {
    const parsed = parseEsportsAsset(assetId);
    if (!parsed) return;
    let match = esportsMatches.find((item) => item.id === parsed.matchId);
    if (!match) {
      toast.error(tr('toast.marketUnavailable'), tr('toast.marketUnavailableDetail'));
      return;
    }
    await handleRefreshOdds(parsed.matchId);
    match = esportsMatches.find((item) => item.id === parsed.matchId);
    if (!match) return;
    const team = match.team1.code === parsed.teamCode ? match.team1 : match.team2.code === parsed.teamCode ? match.team2 : null;
    if (!team || team.priceCents <= 0) {
      toast.error(tr('toast.noQuote'), tr('toast.noQuoteDetail'));
      return;
    }
    await placeEsportsBet(match, team, contracts);
  }

  async function handleAdminLogin(username: string, password: string) {
    const { token } = await adminLogin(username, password);
    adminToken = token;
    user = await fetchMe();
    localStorage.setItem(ADMIN_TOKEN_KEY, token);
    toast.success(tr('toast.signedIn'), tr('toast.adminUnlocked'));
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
      toast.success(tr('toast.loggedIn'), tr('toast.loggedInDetail'));
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
      toast.success(tr('toast.accountCreated'), tr('toast.accountCreatedDetail'));
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
      toast.info(tr('toast.loggedOut'), tr('toast.loggedOutDetail'));
    } finally {
      authBusy = false;
    }
  }

  async function handleUpdateAccount(displayName: string) {
    const next = await updateAccount(displayName);
    user = next;
    toast.success(tr('toast.profileSaved'), next.displayName);
  }

  async function handleChangePassword(currentPassword: string, newPassword: string) {
    await changePassword(currentPassword, newPassword);
    toast.success(tr('toast.passwordChanged'));
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
    toast.success(
      tr('toast.exportCreated'),
      tr(payload.portfolios.length === 1 ? 'toast.exportDetail' : 'toast.exportDetailPlural', { count: payload.portfolios.length })
    );
  }

  async function handleDeletePortfolioData(password: string) {
    await deletePortfolioData(password);
    portfolio = await resetPortfolio(config?.startingCashCents ?? portfolio?.startingCashCents ?? 1_000_000);
    syncMessage = tr('sync.dataDeleted');
    toast.success(tr('toast.portfolioDataDeleted'), tr('toast.portfolioDataDeletedDetail'));
  }

  async function handleDeleteAccount(password: string) {
    await deleteAccount(password);
    user = null;
    adminToken = null;
    localStorage.removeItem(ADMIN_TOKEN_KEY);
    toast.success(tr('toast.accountDeleted'), tr('toast.accountDeletedDetail'));
  }

  async function loadTeams() {
    teamsLoading = true;
    try {
      esportsTeams = await fetchEsportsTeams();
      if (esportsMatches.length > 0) esportsMatches = withLocalTeamImages(esportsMatches);
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
      toast.info(tr('toast.limitReached'), tr('toast.limitReachedDetail', { max: MAX_FAVORITE_TEAMS }));
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
  async function handleRefreshOdds(matchId: string): Promise<boolean> {
    try {
      const fresh = await refreshMatchOdds(matchId);
      esportsMatches = withLocalTeamImages(esportsMatches.map((match) => (match.id === matchId ? fresh : match)));
      await reconcileEsportsPositions();
      return true;
    } catch {
      // Do not leave a stale confirmation bar active after a failed refresh.
      return false;
    }
  }

  // Re-price held esports bet positions to the latest Polymarket odds.
  async function reconcileEsportsPositions() {
    if (!portfolio || esportsMatches.length === 0) return;
    // Online, event positions are priced and settled server-side; a local
    // mark-to-market here would bump updatedAt and block adopting the
    // authoritative snapshot. Offline practice only.
    if (online) return;
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

    // Online: the server prices the fill from its own odds and validates it
    // (authoritative for the competition). Offline falls back to local practice.
    if (online) {
      try {
        const id = clientId || (await loadClientId());
        await pushPracticeState(id);
        const result = await submitEsportsBet(id, {
          portfolioId: PORTFOLIO_ID,
          matchId: match.id,
          teamCode: team.code,
          side: 'buy',
          contracts
        });
        portfolio = result.portfolio;
        openOrders = result.openOrders;
        await savePortfolio(result.portfolio, { touchUpdatedAt: false });
        await saveOpenOrders(result.openOrders);
        toast.success(tr('toast.betPlaced'), tr('toast.betPlacedDetail', { contracts, code: team.code, price: formatMoney(team.priceCents) }));
      } catch (error) {
        toast.error(tr('toast.betFailed'), error instanceof Error ? error.message : undefined);
      }
      return;
    }

    const other = team.code === match.team1.code ? match.team2 : match.team1;
    const grossCents = Math.round(contracts * team.priceCents);
    const feeCents = Math.max(0, Math.round((grossCents * ORDER_FEE_BPS) / 10_000));
    try {
      const next = applyTrade(portfolio, {
        id: crypto.randomUUID(),
        assetId: esportsAssetId(match.id, team.code),
        symbol: team.code,
        name: tr('esports.beatsName', { team: team.name, other: other.code, league: match.league }),
        kind: 'event',
        side: 'buy',
        quantity: contracts,
        priceCents: team.priceCents,
        feeCents
      });
      portfolio = next;
      await savePortfolio(next);
      toast.success(tr('toast.betPlaced'), tr('toast.betPlacedDetail', { contracts, code: team.code, price: formatMoney(team.priceCents) }));
    } catch (error) {
      toast.error(tr('toast.betFailed'), error instanceof Error ? error.message : undefined);
    }
  }

  async function sellEsportsPosition(assetId: string, quantity: number) {
    if (!portfolio) return;
    const position = portfolio.positions.find((item) => item.assetId === assetId);
    if (!position) return;

    // Online: cash out server-side at the current server odds.
    if (online) {
      const parsed = parseEsportsAsset(assetId);
      if (!parsed) return;
      try {
        const id = clientId || (await loadClientId());
        await pushPracticeState(id);
        const result = await submitEsportsBet(id, {
          portfolioId: PORTFOLIO_ID,
          matchId: parsed.matchId,
          teamCode: parsed.teamCode,
          side: 'sell',
          contracts: quantity
        });
        portfolio = result.portfolio;
        openOrders = result.openOrders;
        await savePortfolio(result.portfolio, { touchUpdatedAt: false });
        await saveOpenOrders(result.openOrders);
        toast.success(tr('toast.cashOut'), `${position.symbol}`);
      } catch (error) {
        toast.error(tr('toast.cashOutFailed'), error instanceof Error ? error.message : undefined);
      }
      return;
    }

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
      toast.success(tr('toast.cashOut'), tr('toast.cashOutDetail', { symbol: position.symbol, amount: formatMoney(grossCents - feeCents) }));
    } catch (error) {
      toast.error(tr('toast.cashOutFailed'), error instanceof Error ? error.message : undefined);
    }
  }

  async function handleResetPortfolio() {
    showResetConfirm = false;
    portfolio = await resetPortfolio(config?.startingCashCents ?? portfolio?.startingCashCents ?? 1_000_000);
    openOrders = [];
    await saveOpenOrders([]);
    orderError = '';
    toast.info(tr('toast.portfolioReset'), tr('toast.portfolioResetDetail'));
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
      orderError = tr('order.errPortfolioLoading');
      return;
    }
    if (triggerPriceCents <= 0) {
      orderError = orderType === 'limit' ? tr('order.errInvalidLimit') : tr('order.errInvalidStop');
      return;
    }
    if (!canPlaceOpenOrder) {
      orderError = orderSide === 'sell' ? tr('order.errNotEnoughPosition') : tr('order.errInvalidQuantity');
      return;
    }

    const label = tr('order.previewLabel', {
      type: orderType === 'limit' ? tr('orderType.limit') : tr('orderType.stop'),
      side: orderSide === 'buy' ? tr('side.buyNoun') : tr('side.sellNoun'),
      qty: formatQuantity(normalizedOrderQuantity),
      symbol: selectedMarket.symbol,
      price: formatMoney(triggerPriceCents)
    });

    // Online: queue the order server-side so the backend engine fills it even
    // when this browser is closed (and at the server's own price).
    if (online) {
      try {
        const id = clientId || (await loadClientId());
        await pushPracticeState(id);
        const result = await placeOrder(id, {
          portfolioId: PORTFOLIO_ID,
          assetId: selectedMarket.assetId,
          side: orderSide,
          quantity: normalizedOrderQuantity,
          orderType,
          triggerPriceCents
        });
        portfolio = result.portfolio;
        openOrders = result.openOrders;
        await savePortfolio(result.portfolio, { touchUpdatedAt: false });
        await saveOpenOrders(result.openOrders);
        toast.success(tr('toast.orderQueued'), label);
        orderError = '';
      } catch (error) {
        orderError = error instanceof Error ? error.message : tr('errors.orderNotQueued');
        toast.error(tr('toast.orderFailed'), orderError);
      }
      return;
    }

    // Offline (practice): keep the pending order locally and let the client
    // engine fill it while the app is open.
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
    toast.success(tr('toast.orderQueued'), label);
    orderError = '';

    // Instant fill guard: if the trigger is already satisfied at the current
    // price, execute now so the queue never holds an already-met order.
    void evaluateOpenOrders(markets);
  }

  async function cancelOpenOrder(id: string) {
    if (online) {
      try {
        openOrders = await cancelServerOpenOrder(clientId || (await loadClientId()), PORTFOLIO_ID, id);
        await saveOpenOrders(openOrders);
      } catch (error) {
        toast.error(tr('toast.cancelFailed'), error instanceof Error ? error.message : undefined);
      }
      return;
    }
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
      orderError = tr('order.errPortfolioLoading');
      return;
    }
    if (effectivePriceCents <= 0) {
      orderError = tr('order.errNoMarketPrice');
      return;
    }
    if (!canSubmitOrder) {
      orderError = orderSide === 'buy' ? tr('order.errNotEnoughBuyingPower') : tr('order.errNotEnoughPosition');
      return;
    }

    // Competitive mode: when the backend is reachable, market orders are
    // executed and priced server-side (authoritative) so a client can't
    // fabricate the fill. Only when fully offline do we fall back to the local
    // simulation (unranked practice).
    if (online) {
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
        orderSide === 'buy' ? tr('toast.buyExecuted') : tr('toast.sellExecuted'),
        `${formatQuantity(normalizedOrderQuantity)} ${selectedMarket.symbol} @ ${formatMoney(effectivePriceCents)}`
      );
    } catch (error) {
      orderError = error instanceof Error ? error.message : tr('errors.orderNotPlaced');
      toast.error(tr('toast.orderFailed'), orderError);
    }
  }

  // Reflects the server-side open-order engine: refresh the pending queue, and
  // if any order disappeared (filled/cancelled), adopt the authoritative
  // server portfolio so the new position/cash show up.
  async function refreshServerOrders() {
    if (!online) return;
    try {
      const id = clientId || (await loadClientId());
      const server = await fetchOpenOrders(id, PORTFOLIO_ID);
      const shrank = server.length < openOrders.length;
      openOrders = server;
      await saveOpenOrders(server);

      // An open order just filled server-side → adopt the authoritative
      // portfolio so the new position/cash show up. (Background bet settlement
      // is reflected on the next user action or reload; the server settler keeps
      // the stored state correct in the meantime.)
      if (shrank) {
        const synced = await fetchSyncedPortfolio(id, PORTFOLIO_ID);
        if (synced) {
          portfolio = synced;
          await savePortfolio(synced, { touchUpdatedAt: false });
        }
      }
    } catch {
      // transient; try again next poll
    }
  }

  // Anonymous practice portfolios are client-authoritative, so push the current
  // local state before a server action so the server operates on the latest
  // snapshot. Ranked (logged-in) accounts are server-authoritative — the server
  // ignores any pushed state — so we skip the push entirely.
  async function pushPracticeState(id: string) {
    if (user || !portfolio) return;
    try {
      await syncPortfolio(id, portfolio);
    } catch {
      // non-fatal; the server applies to its last-known snapshot
    }
  }

  // Server-authoritative market order: the returned portfolio is truth.
  async function submitServerOrder() {
    if (!portfolio) return;
    const symbol = selectedMarket.symbol;
    try {
      const id = clientId || (await loadClientId());
      await pushPracticeState(id);
      const result = await placeOrder(id, {
        portfolioId: PORTFOLIO_ID,
        assetId: selectedMarket.assetId,
        side: orderSide,
        quantity: normalizedOrderQuantity
      });
      portfolio = result.portfolio;
      openOrders = result.openOrders;
      await savePortfolio(result.portfolio, { touchUpdatedAt: false });
      await saveOpenOrders(result.openOrders);
      toast.success(
        orderSide === 'buy' ? tr('toast.buyExecuted') : tr('toast.sellExecuted'),
        `${formatQuantity(normalizedOrderQuantity)} ${symbol}`
      );
    } catch (error) {
      orderError = error instanceof Error ? error.message : tr('errors.orderNotPlaced');
      toast.error(tr('toast.orderFailed'), orderError);
    }
  }

  async function handleSyncPortfolio() {
    if (!portfolio) return;
    isSyncing = true;
    try {
      const synced = await syncPortfolio(clientId || (await loadClientId()), portfolio);
      await savePortfolio(synced, { touchUpdatedAt: false });
      portfolio = synced;
      syncMessage = tr('sync.syncedAt', { time: formatUpdatedAt(synced.updatedAt) });
      toast.success(tr('toast.portfolioSynced'));
    } catch (error) {
      syncMessage = tr('sync.failed');
      toast.error(tr('sync.failed'), error instanceof Error ? error.message : undefined);
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
        syncMessage = tr('sync.ready');
        return 'missing';
      }
      const remoteIsNewer = new Date(synced.updatedAt).getTime() > new Date(portfolio.updatedAt).getTime();
      const shouldRestore = remoteIsNewer || (preferRemoteIfPristine && isPristinePortfolio(portfolio));
      if (shouldRestore) {
        await savePortfolio(synced, { touchUpdatedAt: false });
        portfolio = synced;
        syncMessage = tr('sync.restoredAt', { time: formatUpdatedAt(synced.updatedAt) });
        return 'restored';
      }
      syncMessage = tr('sync.localCurrent');
      return 'kept';
    } catch {
      syncMessage = tr('sync.unavailable');
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
          tr('toast.orderExecuted'),
          tr('order.previewLabel', {
            type: order.orderType === 'limit' ? tr('orderType.limit') : tr('orderType.stop'),
            side: order.side === 'buy' ? tr('side.buyNoun') : tr('side.sellNoun'),
            qty: formatQuantity(order.quantity),
            symbol: order.symbol,
            price: formatMoney(price)
          })
        );
      } catch (error) {
        toast.error(tr('toast.orderCanceled'), `${order.symbol}: ${error instanceof Error ? error.message : tr('errors.orderNotFilled')}`);
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

      // Open orders: online, the server engine fills them — just refresh the
      // queue (and adopt the authoritative portfolio if a fill happened).
      // Offline, run the local engine against the fresh prices.
      if (online) {
        await refreshServerOrders();
      } else {
        await evaluateOpenOrders(markets);
      }

      // Background eSports matches updates if relevant
      const hasEsportsPositions = portfolio?.positions.some((p) => p.kind === 'event');
      if (hasEsportsPositions || esportsLoaded) {
        try {
          esportsMatches = withLocalTeamImages(await fetchEsportsMatches());
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
      marketsError = error instanceof Error ? error.message : tr('errors.quoteRefreshFailed');
    }
  }

  function formatUpdatedAt(value: string) {
    const loc = get(locale) === 'de' ? 'de-DE' : 'en-US';
    return new Intl.DateTimeFormat(loc, { hour: '2-digit', minute: '2-digit' }).format(new Date(value));
  }

  // Aging quotes (closed markets / outages) can be days old, so a bare time reads
  // ambiguously — include the date once the quote is no longer from today.
  function formatUpdatedAtFull(value: string) {
    const loc = get(locale) === 'de' ? 'de-DE' : 'en-US';
    const date = new Date(value);
    const sameDay = date.toDateString() === new Date().toDateString();
    if (sameDay) return formatUpdatedAt(value);
    return new Intl.DateTimeFormat(loc, { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' }).format(date);
  }

  function formatChartTime(value: string) {
    const loc = get(locale) === 'de' ? 'de-DE' : 'en-US';
    const date = new Date(value);
    if (chartRange === '1H' || chartRange === '1D') {
      return new Intl.DateTimeFormat(loc, { hour: '2-digit', minute: '2-digit' }).format(date);
    }
    return new Intl.DateTimeFormat(loc, { day: '2-digit', month: 'short' }).format(date);
  }

  function formatQuantity(value: number) {
    const loc = get(locale) === 'de' ? 'de-DE' : 'en-US';
    return new Intl.NumberFormat(loc, { maximumFractionDigits: 6 }).format(value);
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

  function handlePopState() {
    setActiveView(viewFromPath(window.location.pathname), false);
  }

  function setActiveView(view: AppView, updateHistory = true) {
    if (updateHistory && window.location.pathname !== viewPaths[view]) {
      window.history.pushState({}, '', viewPaths[view]);
    }
    activeView = view;
    requestAnimationFrame(() => {
      window.scrollTo(0, 0);
      document.getElementById('main-content')?.focus({ preventScroll: true });
    });
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

  function trapFocus(event: KeyboardEvent) {
    if (event.key !== 'Tab') return;
    const overlay = (event.currentTarget as HTMLElement).querySelector('[role="dialog"]');
    if (!overlay) return;
    const focusable = overlay.querySelectorAll<HTMLElement>('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])');
    if (focusable.length === 0) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  }

</script>

<svelte:window on:keydown={handleKeydown} />
<Toasts />

<a href="#main-content" class="skip-link">{$t('common.skipToContent')}</a>

{#if activeView === 'landing'}
  <main class="landing-shell" id="main-content" tabindex="-1">
    <header class="landing-nav">
      <div class="brand">
        <img src="/icons/koalatrade-icon.png" alt="" width="38" height="38" />
        <div>
          <strong>KoalaTrade</strong>
          <span>Paper markets</span>
        </div>
      </div>
      <button class="nav-action" type="button" title={$t('landing.openDeskTitle')} on:click={() => setActiveView('trade')}>{$t('landing.openDesk')}</button>
    </header>

    <section class="landing-hero" aria-label={$t('landing.intro')}>
      <div class="landing-copy">
        <p class="eyebrow"><Sparkles size={14} /> {$t('landing.eyebrow')}</p>
        <h1>{$t('landing.heading')}</h1>
        <p>
          {$t('landing.body')}
        </p>
        <div class="landing-actions">
          <button class="primary-button" type="button" title={$t('landing.startDeskTitle')} on:click={() => setActiveView('trade')}>{$t('landing.startDesk')}</button>
          <span class:online={config}>{config ? $t('landing.apiReady') : $t('landing.loadingSession')}</span>
        </div>
        <div class="landing-stats">
          <div><strong>{markets.length}</strong><span>{$t('landing.statMarkets')}</span></div>
          <div><strong>{formatMoney(summary.totalEquityCents)}</strong><span>{$t('landing.statEquity')}</span></div>
          <div><strong>0 €</strong><span>{$t('landing.statRisk')}</span></div>
        </div>
      </div>

      <div class="landing-terminal" aria-label={$t('landing.preview')}>
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

    <section class="landing-bands" aria-label={$t('landing.bandsLabel')}>
      <article>
        <CandlestickChart size={20} />
        <strong>{$t('landing.band1Title')}</strong>
        <span>{$t('landing.band1Body')}</span>
      </article>
      <article>
        <WalletCards size={20} />
        <strong>{$t('landing.band2Title')}</strong>
        <span>{$t('landing.band2Body')}</span>
      </article>
      <article>
        <TrendingUp size={20} />
        <strong>{$t('landing.band3Title')}</strong>
        <span>{$t('landing.band3Body')}</span>
      </article>
    </section>
  </main>
{:else}
  <div class="app-shell">
    <nav class="icon-rail" aria-label={$t('nav.main')}>
      <div class="rail-logo" aria-hidden="true"></div>
      {#each deskTabs as tab}
        <button class="rail-item" class:active={activeView === tab.id} type="button"
                title={tab.id === 'trade' ? $t('nav.tradeTitle') :
                       tab.id === 'portfolio' ? $t('nav.portfolioTitle') :
                       tab.id === 'markets' ? $t('nav.marketsTitle') :
                       tab.id === 'esports' ? $t('nav.esportsTitle') : ''}
                on:click={() => setActiveView(tab.id)}>
          <svelte:component this={tab.icon} size={17} />
          <span>{$t(tab.labelKey)}</span>
        </button>
      {/each}
      <button class="rail-item rail-avatar" class:active={activeView === 'profile'} type="button"
              title={$t('nav.profileTitle')}
              on:click={() => setActiveView('profile')}>
        <UserCircle2 size={18} />
        <span>{$t('nav.profile')}</span>
      </button>
    </nav>

  <main class="trading-shell" id="main-content" tabindex="-1">
    <header class="trading-topbar">
      <div class="brand">
        <img src="/icons/koalatrade-icon.png" alt="" width="34" height="34" />
        <div>
          <strong>KoalaTrade</strong>
          <span>{$t('topbar.tagline')}</span>
        </div>
      </div>

      <div class="desk-actions">
        <span class:online={config && !configError} class="status-pill" title={$t('topbar.connTitle')}>
          <i class="dot"></i>{config && !configError ? $t('topbar.apiOnline') : $t('topbar.localMode')}
        </span>
        <div class="lang-switch" role="group" aria-label={$t('common.language')} title={$t('topbar.languageTitle')}>
          {#each LOCALES as loc}
            <button class="lang-option" class:active={$locale === loc} type="button" aria-pressed={$locale === loc} title={LOCALE_LABELS[loc]} on:click={() => setLocale(loc)}>{loc.toUpperCase()}</button>
          {/each}
        </div>
        <button class="icon-button" class:active={activeView === 'admin'} type="button" aria-label={$t('topbar.adminLabel')} title={$t('topbar.adminTitle')} on:click={() => setActiveView('admin')}>
          <ShieldCheck size={18} />
        </button>
        <button class="icon-button" type="button" aria-label={$t('topbar.shortcutsLabel')} title={$t('topbar.shortcutsTitle')} on:click={() => (showShortcuts = !showShortcuts)}>
          <Keyboard size={18} />
        </button>
        <button class="icon-button" type="button" aria-label={$t('topbar.syncLabel')} title={$t('topbar.syncTitle')} disabled={isSyncing || !portfolio || !!configError} on:click={handleSyncPortfolio}>
          <CloudUpload size={18} />
        </button>
        <button class="icon-button" type="button" aria-label={$t('topbar.resetLabel')} title={$t('topbar.resetTitle')} on:click={() => (showResetConfirm = true)}>
          <RotateCcw size={18} />
        </button>
      </div>
    </header>

    <section class="market-tape" aria-label={$t('a11y.marketTape')}>
      {#each markets.slice(0, 6) as item, index}
        <button class:active={selectedMarket && selectedMarket.assetId === item.assetId} type="button" title={$t('tape.selectTitle', { symbol: item.symbol })} on:click={() => selectMarket(item.assetId, true)}>
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
              <strong>{$t('onboarding.new', { amount: formatMoney(config?.startingCashCents ?? 1_000_000) })}</strong>
              <p>{$t('onboarding.pick')}</p>
            </div>
          </div>
          <div class="ob-actions">
            <button type="button" class="ob-tour" title={$t('onboarding.tourTitle')} on:click={() => (showTour = true)}>{$t('onboarding.tour')}</button>
            <button type="button" class="ob-dismiss" title={$t('onboarding.dismissTitle')} on:click={dismissOnboarding}>{$t('onboarding.dismiss')}</button>
          </div>
        </div>
      {/if}
      <section class="trade-layout" aria-label={$t('a11y.tradingWorkspace')}>
        <aside class="watchlist panel" aria-label={$t('nav.markets')}>
          <label class="search compact" aria-label={$t('watchlist.searchLabel')}>
            <Search size={16} />
            <input bind:value={marketQuery} type="search" placeholder={$t('watchlist.searchPlaceholder')} />
          </label>
          <div class="market-filters" aria-label={$t('watchlist.filtersLabel')}>
            {#each marketFilters as filter}
              <button class:active={marketFilter === filter.id} type="button" title={$t('filter.onlyType', { label: $t(filter.labelKey) })} on:click={() => (marketFilter = filter.id)}>{$t(filter.labelKey)}</button>
            {/each}
          </div>
          <div class="watchlist-head"><span>{$t('watchlist.headAsset')}</span><span>{$t('watchlist.headPrice')}</span><span>{$t('watchlist.head24h')}</span></div>
          <div class="market-list">
            {#if marketsLoading}
              {#each Array(6) as _}<div class="skeleton-row"></div>{/each}
            {:else if filteredMarkets.length === 0}
              <p class="empty-state">{$t('watchlist.emptyFilter')}</p>
            {:else}
              {#each filteredMarkets as item}
                <button class:selected={selectedMarket && selectedMarket.assetId === item.assetId} class="market-row" type="button" title={$t('watchlist.selectTitle', { symbol: item.symbol, name: item.name })} on:click={() => selectMarket(item.assetId)}>
                  <span class="asset"><strong>{item.symbol}</strong><small>{item.kind}</small></span>
                  {#if item.priceCents > 0}
                    <span class="price" class:stale={isStalePrice(item)} title={isStalePrice(item) ? $t('watchlist.staleTitle', { time: formatUpdatedAtFull(item.updatedAt) }) : undefined}>{isStalePrice(item) ? '⚠ ' : ''}{formatMoney(item.priceCents)}</span>
                    <em class={marketTone(item.changeBps)}>{formatPercentFromBps(item.changeBps)}</em>
                  {:else}
                    <span class="no-feed" title={$t('watchlist.noFeedTitle')}><i></i>{$t('watchlist.noFeed')}</span>
                  {/if}
                </button>
              {/each}
            {/if}
          </div>
        </aside>

        <section class="market-stage">
          <section class="instrument-strip panel" aria-label={$t('a11y.selectedMarket')}>
            {#if selectedMarket}
              <div class="instrument-id">
                <p class="eyebrow">{selectedMarket.kind} · {selectedMarket.source}</p>
                <h1>{selectedMarket.symbol}</h1>
                <span>{selectedMarket.name}</span>
              </div>
              <div class="instrument-price">
                <strong>{formatPrice(selectedMarket.priceCents)}</strong>
                <span class={selectedMarket.priceCents > 0 ? marketTone(selectedMarket.changeBps) : ''}>
                  {selectedMarket.priceCents > 0 ? formatPercentFromBps(selectedMarket.changeBps) + ' ' + $t('instrument.today') : '—'}
                </span>
              </div>
            {:else}
              <div class="instrument-id">
                <p class="eyebrow">—</p>
                <h1>{$t('instrument.noMarket')}</h1>
              </div>
            {/if}
            <div class="instrument-stats">
              <span>{$t('instrument.equity')} <strong>{formatMoney(summary.totalEquityCents)}</strong></span>
              <span>{$t('instrument.cash')} <strong>{formatMoney(summary.cashCents)}</strong></span>
              <span>{$t('instrument.return')} <strong class={changeColor(summary.totalReturnBps)}>{formatPercentFromBps(summary.totalReturnBps)}</strong></span>
            </div>
          </section>

          <section class="chart-panel panel" aria-label={$t('a11y.priceChart')}>
            <div class="chart-toolbar">
              <div>
                <p class="eyebrow">{$t('chart.eyebrow')} · {chartRange}</p>
                {#if selectedMarket && selectedMarket.priceCents > 0}
                  <h2>{formatMoney(selectedMarket.priceCents)} <em class={changeColor(rangeChangeBps)}>{formatSignedMoney(rangeChangeCents)} ({formatPercentFromBps(rangeChangeBps)})</em></h2>
                {:else}
                  <h2>—</h2>
                {/if}
              </div>
              <div class="chart-controls">
                <button class="sma-toggle" class:active={showSma} type="button" title={$t('chart.smaTitle')} on:click={() => (showSma = !showSma)}>SMA 14</button>
                <InfoTip placement="bottom" text={$t('chart.smaTip')} />

                <div class="timeframes">
                  {#each chartRanges as range}
                    <button class:active={chartRange === range} type="button" title={$t('chart.rangeTitle', { range })} on:click={() => (chartRange = range)}>{range}</button>
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
              <span>{$t('chart.open')} <strong>{formatMoney(chartOpen)}</strong></span>
              <span>{$t('chart.high')} <strong>{formatMoney(chartHigh)}</strong></span>
              <span>{$t('chart.low')} <strong>{formatMoney(chartLow)}</strong></span>
              <span>{$t('chart.range')}<InfoTip placement="bottom" text={$t('chart.rangeTip')} /> <strong>{formatPercentFromBps(chartLow > 0 ? Math.round(((chartHigh - chartLow) / chartLow) * 10_000) : 0)}</strong></span>
            </div>
          </section>
        </section>

        <aside class="execution-column" aria-label={$t('a11y.execution')}>
          <section class="panel market-detail" aria-label={$t('detail.title')}>
            <div class="panel-head"><div><p class="eyebrow">Live · {selectedMarket.source || '—'}</p><h2>{$t('detail.title')}</h2></div><Activity size={18} /></div>
            <div class="detail-grid">
              <div><span>{$t('detail.price')}</span><strong>{formatPrice(selectedMarket.priceCents)}</strong></div>
              <div><span>{$t('detail.change24h')}</span><strong class={selectedMarket.priceCents > 0 ? changeColor(selectedMarket.changeBps) : ''}>{selectedMarket.priceCents > 0 ? formatPercentFromBps(selectedMarket.changeBps) : '—'}</strong></div>
              <div><span>{$t('detail.type')}</span><strong>{selectedMarket.kind}</strong></div>
              <div><span>{$t('detail.updated')}</span>
                {#if selectedMarketFreshness === 'stale'}
                  <strong class="stale" title={$t('detail.staleTitle', { time: formatUpdatedAtFull(selectedMarket.updatedAt) })}>⚠ {formatUpdatedAtFull(selectedMarket.updatedAt)}</strong>
                {:else if selectedMarketFreshness === 'closed'}
                  <strong class="closed" title={$t('detail.closedTitle', { time: formatUpdatedAtFull(selectedMarket.updatedAt) })}>🌙 {formatUpdatedAtFull(selectedMarket.updatedAt)}</strong>
                {:else}
                  <strong>{selectedMarket.updatedAt ? formatUpdatedAt(selectedMarket.updatedAt) : '—'}</strong>
                {/if}
              </div>
            </div>
            {#if selectedPositionRow}
              <div class="detail-position">
                <p class="eyebrow">{$t('detail.yourPosition')}</p>
                <div class="detail-grid">
                  <div><span>{$t('detail.quantity')}</span><strong>{formatQuantity(selectedPositionRow.quantity)}</strong></div>
                  <div><span>{$t('detail.avgCost')}<InfoTip text={$t('detail.avgCostTip')} /></span><strong>{formatMoney(selectedPositionRow.averageCostCents)}</strong></div>
                  <div><span>{$t('detail.marketValue')}</span><strong>{formatMoney(selectedPositionRow.marketValueCents)}</strong></div>
                  <div><span>{$t('detail.pnl')}<InfoTip text={$t('detail.pnlTip')} /></span><strong class={changeColor(selectedPositionRow.pnlCents)}>{formatSignedMoney(selectedPositionRow.pnlCents)}</strong></div>
                </div>
              </div>
            {:else}
              <p class="panel-note">{$t('detail.noPosition', { symbol: selectedMarket.symbol })}</p>
            {/if}
          </section>

          <section class="order-panel panel" aria-label={$t('order.ticket')}>
            <div class="panel-head">
              <div><p class="eyebrow">{$t('order.ticket')} · {orderType === 'market' ? $t('orderType.market') : orderType === 'limit' ? $t('orderType.limit') : $t('orderType.stop')}</p><h2>{orderSide === 'buy' ? $t('side.buyVerb') : $t('side.sellVerb')} {selectedMarket.symbol}</h2></div>
              <Zap size={18} />
            </div>
            <form class="order-form" on:submit|preventDefault={handleSubmitOrder}>
              <div class="segmented" aria-label={$t('order.sideLabel')}>
                <button class:active={orderSide === 'buy'} type="button" title={$t('order.buyTitle')} on:click={() => setOrderSide('buy')}>{$t('side.buyVerb')}</button>
                <button class:active={orderSide === 'sell'} class="sell" type="button" title={$t('order.sellTitle')} on:click={() => setOrderSide('sell')}>{$t('side.sellVerb')}</button>
              </div>

              <div class="order-types" role="tablist" aria-label={$t('order.typeLabel')}>
                <button class:active={orderType === 'market'} type="button" role="tab" aria-selected={orderType === 'market'} title={$t('order.marketTitle')} on:click={() => setOrderType('market')}>{$t('orderType.market')}</button>
                <button class:active={orderType === 'limit'} type="button" role="tab" aria-selected={orderType === 'limit'} title={$t('order.limitTitle')} on:click={() => setOrderType('limit')}>{$t('orderType.limit')}</button>
                <button class:active={orderType === 'stop'} type="button" role="tab" aria-selected={orderType === 'stop'} title={$t('order.stopTitle')} on:click={() => setOrderType('stop')}>{$t('orderType.stop')}</button>
              </div>

              <p class="order-hint">{orderTypeHint}</p>

              <label class="field" title={$t('order.quantityTitle')}>
                <span>{$t('order.quantity')}</span>
                <input bind:value={orderQuantity} min="0.0001" step="0.0001" type="number" title={$t('order.quantityInputTitle')} />
              </label>

              {#if orderType === 'limit'}
                <label class="field trigger limit" title={$t('order.limitPriceFieldTitle')}>
                  <span>{$t('order.limitPrice')} <small>{$t('order.limitPriceHint')}</small></span>
                  <input bind:value={triggerPrice} min="0" step="0.01" type="number" placeholder={(effectivePriceCents / 100).toFixed(2)} title={$t('order.limitPriceInputTitle')} />
                </label>
              {:else if orderType === 'stop'}
                <label class="field trigger stop" title={$t('order.stopPriceFieldTitle')}>
                  <span>{$t('order.stopPrice')} <small>{$t('order.stopPriceHint')}</small></span>
                  <input bind:value={triggerPrice} min="0" step="0.01" type="number" placeholder={(effectivePriceCents / 100).toFixed(2)} title={$t('order.stopPriceInputTitle')} />
                </label>
              {/if}

              {#if orderType === 'market'}
                <div class="presets" aria-label={$t('order.presetsLabel')}>
                  {#each quantityPresets as preset}
                    <button type="button" disabled={orderLimitQuantity <= 0} title={$t('order.presetTitle', { pct: Math.round(preset * 100) })} on:click={() => applyPreset(preset)}>{Math.round(preset * 100)}%</button>
                  {/each}
                </div>
              {/if}

              <div class="order-power"><span>{orderPowerLabel}</span><strong>{orderPowerValue}</strong></div>

              <div class="order-summary">
                {#if isOpenOrderType}
                  <div title={$t('order.triggerTitle')}><span>{orderType === 'limit' ? $t('order.limitPrice') : $t('order.stopPrice')}</span><strong>{triggerPriceCents > 0 ? formatMoney(triggerPriceCents) : '—'}</strong></div>
                  <div title={$t('order.volumeTitle')}><span>{$t('order.volume')}</span><strong>{triggerPriceCents > 0 ? formatMoney(Math.round(normalizedOrderQuantity * triggerPriceCents)) : '—'}</strong></div>
                {:else}
                  <div title={$t('order.marketPriceTitle')}><span>{$t('order.marketPrice')}</span><strong>{formatMoney(effectivePriceCents)}</strong></div>
                  <div title={$t('order.grossValueTitle')}><span>{$t('order.grossValue')}</span><strong>{formatMoney(estimatedOrderValue)}</strong></div>
                  <div title={$t('order.feeTitle')}><span>{$t('order.fee', { pct: (ORDER_FEE_BPS / 100).toFixed(2) })}<InfoTip text={$t('order.feeTip', { pct: (ORDER_FEE_BPS / 100).toFixed(2) })} /></span><strong>{formatMoney(estimatedOrderFee)}</strong></div>
                  <div class="total" title={$t('order.totalTitle')}><span>{orderSide === 'buy' ? $t('order.cashDebit') : $t('order.cashCredit')}</span><strong>{formatMoney(estimatedOrderTotal)}</strong></div>
                {/if}
                <div class="status-row" title={$t('order.statusTitle')}><span>{orderStatusLabel}</span><strong>{orderStatusValue}</strong></div>
              </div>

              {#if orderError}<p class="form-error">{orderError}</p>{/if}
              <button class="primary-button" class:danger={orderSide === 'sell'} type="submit" title={isOpenOrderType ? $t('order.queueTitle') : orderSide === 'buy' ? $t('order.submitBuyTitle', { qty: normalizedOrderQuantity, symbol: selectedMarket.symbol }) : $t('order.submitSellTitle', { qty: normalizedOrderQuantity, symbol: selectedMarket.symbol })} disabled={!canPlaceOrder}>
                {submitLabel}
              </button>
            </form>
          </section>

          {#if openOrders.length > 0}
            <section class="panel open-orders" aria-label={$t('openOrders.label')}>
              <div class="panel-head"><div><p class="eyebrow">{$t('openOrders.queue')}</p><h2>{$t('openOrders.heading', { count: openOrders.length })}</h2></div><Activity size={18} /></div>
              <div class="open-orders-list">
                {#each openOrders as order (order.id)}
                  <div class="open-order-row" class:this-asset={order.assetId === selectedMarket.assetId}>
                    <div class="oo-id">
                      <strong>{order.orderType === 'limit' ? $t('orderType.limit') : $t('orderType.stop')} {order.side === 'buy' ? $t('side.buyNoun') : $t('side.sellNoun')}</strong>
                      <small>{formatQuantity(order.quantity)} {order.symbol} @ {formatMoney(order.triggerPriceCents)}</small>
                    </div>
                    <span class="oo-status">{$t('openOrders.waiting')}</span>
                    <button type="button" class="oo-cancel" title={$t('openOrders.cancelTitle')} on:click={() => cancelOpenOrder(order.id)}>{$t('openOrders.cancel')}</button>
                  </div>
                {/each}
              </div>
            </section>
          {/if}
        </aside>
      </section>
    {:else if activeView === 'portfolio'}
      <section class="view-scroll" aria-label={$t('nav.portfolio')}>
        <section class="portfolio-metrics">
          <div class="metric primary">
            <span>{$t('portfolio.equity')}<InfoTip placement="bottom" text={$t('portfolio.equityTip')} /></span>
            <strong>{formatMoney(summary.totalEquityCents)}</strong>
            <em class={changeColor(summary.totalReturnBps)}>{formatSignedMoney(summary.totalReturnCents)} ({formatPercentFromBps(summary.totalReturnBps)})</em>
          </div>
          <div class="metric"><span>{$t('portfolio.cash')}</span><strong>{formatMoney(summary.cashCents)}</strong><em>{$t('portfolio.positionsCount', { count: summary.openPositions })}</em></div>
          <div class="metric"><span>{$t('portfolio.realizedPnl')}<InfoTip placement="bottom" text={$t('portfolio.realizedTip')} /></span><strong class={changeColor(performance.realizedPnlCents)}>{formatSignedMoney(performance.realizedPnlCents)}</strong><em>{$t('portfolio.closed')}</em></div>
          <div class="metric"><span>{$t('portfolio.unrealized')}<InfoTip placement="bottom" text={$t('portfolio.unrealizedTip')} /></span><strong class={changeColor(performance.unrealizedPnlCents)}>{formatSignedMoney(performance.unrealizedPnlCents)}</strong><em>{$t('portfolio.open')}</em></div>
          <div class="metric"><span>{$t('portfolio.maxDrawdown')}<InfoTip placement="bottom" align="right" text={$t('portfolio.drawdownTip')} /></span><strong class:down={performance.drawdownBps > 0}>{formatPercentFromBps(-performance.drawdownBps)}</strong><em>{$t('portfolio.peak', { amount: formatMoney(performance.peakEquityCents) })}</em></div>
        </section>

        <section class="panel" aria-label={$t('portfolio.equityCurve')}>
          <div class="panel-head"><div><p class="eyebrow">{$t('portfolio.performance')}</p><h2>{$t('portfolio.equityCurve')}</h2></div><LineChart size={18} /></div>
          <AreaChart
            series={performance.curve.map((point) => point.equityCents)}
            labels={performance.curve.map((point) => point.t)}
            height={260}
            accent={summary.totalReturnCents >= 0 ? 'up' : 'down'}
            formatValue={formatMoney}
            formatLabel={(value) => { const loc = get(locale) === 'de' ? 'de-DE' : 'en-US'; return new Intl.DateTimeFormat(loc, { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' }).format(new Date(value)); }}
          />
        </section>

        <div class="portfolio-grid">
          <section class="panel" aria-label={$t('portfolio.positions')}>
            <div class="panel-head">
              <div><p class="eyebrow">{$t('portfolio.holdings')}</p><h2>{$t('portfolio.positions')}</h2></div>
              <div class="mini-toggle">
                <button class:active={positionSort === 'value'} type="button" title={$t('portfolio.sortValueTitle')} on:click={() => (positionSort = 'value')}>{$t('portfolio.sortValue')}</button>
                <button class:active={positionSort === 'pnl'} type="button" title={$t('portfolio.sortPnlTitle')} on:click={() => (positionSort = 'pnl')}>{$t('portfolio.sortPnl')}</button>
              </div>
            </div>
            <div class="table">
              <div class="table-head pos"><span>{$t('watchlist.headAsset')}</span><span>{$t('detail.quantity')}</span><span>{$t('portfolio.headValue')}</span><span>P&L</span></div>
              {#if sortedPositionRows.length === 0}
                <p class="empty-state">{$t('portfolio.emptyPositions')}</p>
              {:else}
                {#each sortedPositionRows as position}
                  <button class="table-row pos" type="button" title={$t('portfolio.openPositionTitle', { symbol: position.symbol })} on:click={() => selectMarket(position.assetId, true)}>
                    <span class="asset"><strong>{position.symbol}</strong><small>Ø {formatMoney(position.averageCostCents)}</small></span>
                    <span>{formatQuantity(position.quantity)}</span>
                    <span>{formatMoney(position.marketValueCents)}</span>
                    <em class={changeColor(position.pnlCents)}>{formatSignedMoney(position.pnlCents)}<small>{formatPercentFromBps(position.pnlBps)}</small></em>
                  </button>
                {/each}
              {/if}
            </div>
          </section>

          <section class="panel" aria-label={$t('portfolio.history')}>
            <div class="panel-head"><div><p class="eyebrow">{$t('portfolio.history')}</p><h2>{$t('portfolio.orders')}</h2></div><Activity size={18} /></div>
            <div class="table">
              <div class="table-head ord"><span>{$t('portfolio.headOrder')}</span><span>{$t('portfolio.headFill')}</span><span>{$t('portfolio.headStatus')}</span></div>
              {#if (portfolio?.transactions.length ?? 0) === 0}
                <p class="empty-state">{$t('portfolio.emptyTrades')}</p>
              {:else}
                {#each portfolio?.transactions.slice(0, 18) ?? [] as tx}
                  <div class="table-row ord">
                    <strong class={tx.side}>{tx.side === 'buy' ? $t('portfolio.txBuy') : $t('portfolio.txSell')} {tx.symbol}<small>{formatUpdatedAt(tx.createdAt)}</small></strong>
                    <span>{formatQuantity(tx.quantity)} @ {formatMoney(tx.priceCents)}<small>{$t('portfolio.feeShort', { amount: formatMoney(tx.feeCents) })}</small></span>
                    <em class={tx.status === 'synced' ? 'synced-tag' : 'local-tag'}>{tx.status === 'synced' ? $t('portfolio.synced') : $t('portfolio.local')}</em>
                  </div>
                {/each}
              {/if}
            </div>
          </section>
        </div>
      </section>
    {:else if activeView === 'markets'}
      <section class="view-scroll" aria-label={$t('nav.markets')}>
        <div class="markets-toolbar panel">
          <label class="search compact" aria-label={$t('watchlist.searchLabel')}>
            <Search size={16} />
            <input bind:value={marketQuery} type="search" placeholder={$t('watchlist.searchPlaceholder')} />
          </label>
          <div class="market-filters wide">
            {#each marketFilters as filter}
              <button class:active={marketFilter === filter.id} type="button" title={$t('filter.onlyType', { label: $t(filter.labelKey) })} on:click={() => (marketFilter = filter.id)}>{$t(filter.labelKey)}</button>
            {/each}
          </div>
        </div>

        <div class="market-grid">
          {#if marketsLoading}
            {#each Array(6) as _}<div class="market-card skeleton"></div>{/each}
          {:else if filteredMarkets.length === 0}
            <p class="empty-state">{$t('watchlist.emptyFilter')}</p>
          {:else}
            {#each filteredMarkets as item}
              <button class="market-card" class:selected={selectedMarket.assetId === item.assetId} type="button" title={$t('markets.openDeskTitle', { symbol: item.symbol, name: item.name })} on:click={() => selectMarket(item.assetId, true)}>
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
      <section class="view-scroll" aria-label={$t('nav.esports')}>
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
    {:else if activeView === 'leaderboard'}
      <section class="view-scroll" aria-label={$t('nav.leaderboard')}>
        <LeaderboardView
          entries={leaderboard}
          loading={leaderboardLoading}
          error={leaderboardError}
          {user}
          onRefresh={() => { leaderboardLoaded = false; void loadLeaderboard(); }}
          onGoToProfile={() => setActiveView('profile')}
        />
      </section>
    {:else if activeView === 'profile'}
      <section class="view-scroll" aria-label={$t('nav.profile')}>
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
      <section class="view-scroll" aria-label={$t('topbar.adminLabel')}>
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
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="shortcuts-overlay" on:keydown={trapFocus}>
      <button class="shortcuts-backdrop" type="button" aria-label={$t('shortcuts.closeLabel')} title={$t('shortcuts.closeTitle')} on:click={() => (showShortcuts = false)}></button>
      <div class="shortcuts-card" role="dialog" aria-label={$t('shortcuts.title')} aria-modal="true">
        <div class="panel-head"><div><p class="eyebrow">{$t('shortcuts.help')}</p><h2>{$t('shortcuts.title')}</h2></div><Keyboard size={18} /></div>
        <ul>
          <li><kbd>B</kbd><span>{$t('shortcuts.buySide')}</span></li>
          <li><kbd>S</kbd><span>{$t('shortcuts.sellSide')}</span></li>
          <li><kbd>1</kbd>–<kbd>6</kbd><span>{$t('shortcuts.pickMarket')}</span></li>
          <li><kbd>?</kbd><span>{$t('shortcuts.thisHelp')}</span></li>
        </ul>
        <button class="primary-button" type="button" title={$t('shortcuts.closeButtonTitle')} on:click={() => (showShortcuts = false)}>{$t('common.close')}</button>
      </div>
    </div>
  {/if}

  {#if showResetConfirm}
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="shortcuts-overlay" on:keydown={trapFocus}>
      <button class="shortcuts-backdrop" type="button" aria-label={$t('common.close')} title={$t('reset.closeTitle')} on:click={() => (showResetConfirm = false)}></button>
      <div class="shortcuts-card" role="dialog" aria-label={$t('topbar.resetLabel')} aria-modal="true">
        <div class="panel-head"><div><p class="eyebrow">{$t('reset.confirm')}</p><h2>{$t('reset.heading')}</h2></div><RotateCcw size={18} /></div>
        <p class="confirm-text">{$t('reset.text', { amount: formatMoney(portfolio?.startingCashCents ?? config?.startingCashCents ?? 1_000_000) })}</p>
        <div class="confirm-actions">
          <button class="ghost-button" type="button" title={$t('reset.cancelTitle')} on:click={() => (showResetConfirm = false)}>{$t('common.cancel')}</button>
          <button class="primary-button danger" type="button" title={$t('reset.confirmTitle')} on:click={handleResetPortfolio}>{$t('reset.confirmButton')}</button>
        </div>
      </div>
    </div>
  {/if}

  {#if showTour}
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="shortcuts-overlay" on:keydown={trapFocus}>
      <button class="shortcuts-backdrop" type="button" aria-label={$t('common.close')} title={$t('tour.closeTitle')} on:click={() => (showTour = false)}></button>
      <div class="shortcuts-card onboarding-card" role="dialog" aria-label={$t('tour.welcome')} aria-modal="true">
        <div class="panel-head"><div><p class="eyebrow">{$t('tour.welcome')}</p><h2>{$t('tour.heading')}</h2></div><Sparkles size={18} /></div>
        <ul class="onboarding-list">
          <li><WalletCards size={16} /><span>{$t('tour.point1Before')}<strong>{formatMoney(config?.startingCashCents ?? 1_000_000)}</strong>{$t('tour.point1After')}<strong>{$t('tour.point1Risk')}</strong>.</span></li>
          <li><CandlestickChart size={16} /><span>{$t('tour.point2')}</span></li>
          <li><Trophy size={16} /><span>{$t('tour.point3Before')}<strong>{$t('tour.point3Esports')}</strong>{$t('tour.point3After', { amount: formatMoney(10_000) })}</span></li>
          <li><UserCircle2 size={16} /><span>{$t('tour.point4')}</span></li>
        </ul>
        <button class="primary-button" type="button" title={$t('tour.startTitle')} on:click={dismissOnboarding}>{$t('tour.start')}</button>
      </div>
    </div>
  {/if}
{/if}
