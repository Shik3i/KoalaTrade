import type { AssetKind, OpenOrder, PortfolioSnapshot } from './portfolio';

export type PublicConfig = {
  appName: string;
  environment: string;
  startingCashCents: number;
  marketDataSource: string;
  registrationOpen: boolean;
};

export type SessionUser = {
  id: string;
  username: string;
  displayName: string;
  role: 'user' | 'admin';
};

export type AccountExport = {
  exportedAt: string;
  user: SessionUser;
  portfolios: PortfolioSnapshot[];
};

export type Market = {
  assetId: string;
  symbol: string;
  name: string;
  kind: AssetKind;
  source: string;
  priceCents: number;
  changeBps: number;
  updatedAt: string;
};

export type MarketsResponse = {
  markets: Market[];
};

export type Quote = {
  assetId: string;
  symbol: string;
  priceCents: number;
  changeBps: number;
  source: string;
  updatedAt: string;
  cachedUntil: string;
};

export type ChartRange = '1H' | '1D' | '1W' | '1M' | '1Y';

export type Candle = {
  time: string;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
};

export type MarketHistory = {
  assetId: string;
  range: ChartRange;
  candles: Candle[];
};

// ---- Admin ----
export type TeamMapping = {
  originalCode: string;
  polymarketCode: string;
  updatedAt: string;
};

export type SlugDiagnostic = {
  match: EsportsMatch;
  slugs: string[];
  found: boolean;
  eventSlug: string;
  polymarketUrl: string;
};

export type AdminStatus = {
  esports: {
    scheduleCached: boolean;
    scheduleAgeSeconds: number;
    matchCount: number;
    matchesWithOdds: number;
    resultsCount: number;
    teamsCached: boolean;
    teamCount: number;
  };
  marketDataSource: string;
};

export type AdminSettings = {
  registrationOpen: boolean;
};

type LoginPayload = { token?: string; expiresAt: string; user: SessionUser };

export async function login(username: string, password: string): Promise<LoginPayload> {
  const response = await fetch('/api/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify({ username, password })
  });
  if (response.status === 401) throw new Error('Falsche Zugangsdaten');
  if (response.status === 429) throw new Error('Zu viele Versuche. Bitte später erneut probieren.');
  if (!response.ok) throw new Error(`Login fehlgeschlagen (${response.status})`);
  return (await response.json()) as LoginPayload;
}

export async function register(username: string, password: string): Promise<LoginPayload> {
  const response = await fetch('/api/auth/register', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify({ username, password })
  });
  if (response.status === 403) throw new Error('Registrierung ist aktuell geschlossen');
  if (response.status === 409) throw new Error('Benutzername ist bereits vergeben');
  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as { error?: string } | null;
    throw new Error(payload?.error ?? `Registrierung fehlgeschlagen (${response.status})`);
  }
  return (await response.json()) as LoginPayload;
}

export async function logout(): Promise<void> {
  const response = await fetch('/api/auth/logout', { method: 'POST', headers: { Accept: 'application/json' } });
  if (!response.ok) throw new Error(`Logout fehlgeschlagen (${response.status})`);
}

export async function fetchMe(): Promise<SessionUser | null> {
  const response = await fetch('/api/auth/me', { headers: { Accept: 'application/json' } });
  if (response.status === 401) return null;
  if (!response.ok) throw new Error(`Session request failed with ${response.status}`);
  return ((await response.json()) as { user: SessionUser }).user;
}

async function accountJson<T>(response: Response): Promise<T> {
  if (response.status === 401) throw new Error('Sitzung abgelaufen oder Passwort falsch');
  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as { error?: string } | null;
    throw new Error(payload?.error ?? `Request fehlgeschlagen (${response.status})`);
  }
  return (await response.json()) as T;
}

export async function updateAccount(displayName: string): Promise<SessionUser> {
  const response = await fetch('/api/account/', {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify({ displayName })
  });
  return (await accountJson<{ user: SessionUser }>(response)).user;
}

export async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
  const response = await fetch('/api/account/password', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify({ currentPassword, newPassword })
  });
  await accountJson<{ ok: boolean }>(response);
}

export async function exportAccount(): Promise<AccountExport> {
  const response = await fetch('/api/account/export', { headers: { Accept: 'application/json' } });
  return accountJson<AccountExport>(response);
}

export async function deletePortfolioData(password: string): Promise<void> {
  const response = await fetch('/api/account/portfolio-data', {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify({ password })
  });
  await accountJson<{ ok: boolean }>(response);
}

export async function deleteAccount(password: string): Promise<void> {
  const response = await fetch('/api/account/', {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify({ password })
  });
  await accountJson<{ ok: boolean }>(response);
}

export async function adminLogin(username: string, password: string): Promise<{ token: string; expiresAt: string }> {
  const payload = await login(username, password);
  if (!payload.token) throw new Error('Admin-Rolle erforderlich');
  return { token: payload.token, expiresAt: payload.expiresAt };
}

function adminHeaders(token: string) {
  return { Authorization: `Bearer ${token}`, Accept: 'application/json' };
}

export class AdminAuthError extends Error {}

async function adminJson<T>(response: Response): Promise<T> {
  if (response.status === 401) throw new AdminAuthError('Sitzung abgelaufen');
  if (!response.ok) throw new Error(`Request fehlgeschlagen (${response.status})`);
  return (await response.json()) as T;
}

export async function fetchTeamMappings(token: string): Promise<TeamMapping[]> {
  const response = await fetch('/api/admin/mappings', { headers: adminHeaders(token) });
  return (await adminJson<{ mappings: TeamMapping[] }>(response)).mappings ?? [];
}

export async function upsertTeamMapping(token: string, originalCode: string, polymarketCode: string): Promise<TeamMapping[]> {
  const response = await fetch('/api/admin/mappings', {
    method: 'PUT',
    headers: { ...adminHeaders(token), 'Content-Type': 'application/json' },
    body: JSON.stringify({ originalCode, polymarketCode })
  });
  return (await adminJson<{ mappings: TeamMapping[] }>(response)).mappings ?? [];
}

export async function deleteTeamMapping(token: string, originalCode: string): Promise<TeamMapping[]> {
  const response = await fetch(`/api/admin/mappings/${encodeURIComponent(originalCode)}`, {
    method: 'DELETE',
    headers: adminHeaders(token)
  });
  return (await adminJson<{ mappings: TeamMapping[] }>(response)).mappings ?? [];
}

export async function previewTeamMapping(
  token: string,
  input: { matchId: string; originalCode: string; polymarketCode: string; liveTest: boolean }
): Promise<SlugDiagnostic> {
  const response = await fetch('/api/admin/slug-preview', {
    method: 'POST',
    headers: { ...adminHeaders(token), 'Content-Type': 'application/json' },
    body: JSON.stringify(input)
  });
  return adminJson<SlugDiagnostic>(response);
}

export async function fetchAdminStatus(token: string): Promise<AdminStatus> {
  const response = await fetch('/api/admin/status', { headers: adminHeaders(token) });
  return adminJson<AdminStatus>(response);
}

export async function fetchAdminSettings(token: string): Promise<AdminSettings> {
  const response = await fetch('/api/admin/settings', { headers: adminHeaders(token) });
  return adminJson<AdminSettings>(response);
}

export async function updateAdminSettings(token: string, settings: AdminSettings): Promise<AdminSettings> {
  const response = await fetch('/api/admin/settings', {
    method: 'PUT',
    headers: { ...adminHeaders(token), 'Content-Type': 'application/json' },
    body: JSON.stringify(settings)
  });
  return adminJson<AdminSettings>(response);
}

export async function adminRefreshEsports(token: string): Promise<number> {
  const response = await fetch('/api/admin/refresh', { method: 'POST', headers: adminHeaders(token) });
  return (await adminJson<{ refreshed: number }>(response)).refreshed ?? 0;
}

export async function fetchPublicConfig(): Promise<PublicConfig> {
  const response = await fetch('/api/config', {
    headers: { Accept: 'application/json' }
  });

  if (!response.ok) {
    throw new Error(`Config request failed with ${response.status}`);
  }

  return response.json() as Promise<PublicConfig>;
}

export async function fetchMarkets(): Promise<Market[]> {
  const response = await fetch('/api/markets', {
    headers: { Accept: 'application/json' }
  });

  if (!response.ok) {
    throw new Error(`Markets request failed with ${response.status}`);
  }

  const payload = (await response.json()) as MarketsResponse;
  return payload.markets;
}

export type EsportsTeam = {
  name: string;
  code: string;
  image: string;
  probBps: number;
  priceCents: number;
};

export type EsportsMatch = {
  id: string;
  startTime: string;
  state: string;
  league: string;
  blockName: string;
  bestOf: number;
  team1: EsportsTeam;
  team2: EsportsTeam;
  hasOdds: boolean;
  polymarketUrl: string;
};

export type EsportsTeamInfo = {
  code: string;
  name: string;
  league: string;
  image: string;
};

export async function fetchEsportsMatches(): Promise<EsportsMatch[]> {
  const response = await fetch('/api/esports/matches', {
    headers: { Accept: 'application/json' }
  });

  if (!response.ok) {
    throw new Error(`Esports request failed with ${response.status}`);
  }

  const payload = (await response.json()) as { matches: EsportsMatch[] };
  return payload.matches ?? [];
}

export async function fetchEsportsTeams(): Promise<EsportsTeamInfo[]> {
  const response = await fetch('/api/esports/teams', {
    headers: { Accept: 'application/json' }
  });

  if (!response.ok) {
    throw new Error(`Esports teams request failed with ${response.status}`);
  }

  const payload = (await response.json()) as { teams: EsportsTeamInfo[] };
  return payload.teams ?? [];
}

export type EsportsResult = {
  matchId: string;
  winnerCode: string;
  team1Code: string;
  team2Code: string;
  completedAt: string;
};

// Settled outcomes for the given match ids (only completed ones are returned),
// used to auto-resolve open bets.
export async function fetchEsportsResults(matchIds: string[]): Promise<EsportsResult[]> {
  if (matchIds.length === 0) return [];
  const params = new URLSearchParams({ ids: matchIds.join(',') });
  const response = await fetch(`/api/esports/results?${params}`, {
    headers: { Accept: 'application/json' }
  });

  if (!response.ok) {
    throw new Error(`Esports results request failed with ${response.status}`);
  }

  const payload = (await response.json()) as { results: EsportsResult[] };
  return payload.results ?? [];
}

// Force-refresh a single match's Polymarket odds on demand (no rate limit),
// called right before placing a bet so the user sees the freshest price.
export async function refreshMatchOdds(matchId: string): Promise<EsportsMatch> {
  const response = await fetch(`/api/esports/matches/${encodeURIComponent(matchId)}/odds`, {
    headers: { Accept: 'application/json' }
  });

  if (!response.ok) {
    throw new Error(`Odds refresh failed with ${response.status}`);
  }

  return (await response.json()) as EsportsMatch;
}

export async function fetchMarketHistory(
  assetId: string,
  range: ChartRange
): Promise<Candle[]> {
  const params = new URLSearchParams({ range });
  const response = await fetch(`/api/markets/${encodeURIComponent(assetId)}/history?${params}`, {
    headers: { Accept: 'application/json' }
  });

  if (!response.ok) {
    throw new Error(`History request failed with ${response.status}`);
  }

  const payload = (await response.json()) as MarketHistory;
  return payload.candles;
}

export async function fetchQuotes(assetIds: string[]): Promise<Quote[]> {
  const params = new URLSearchParams({ ids: assetIds.join(',') });
  const response = await fetch(`/api/quotes?${params}`, {
    headers: { Accept: 'application/json' }
  });

  if (!response.ok) {
    throw new Error(`Quotes request failed with ${response.status}`);
  }

  const payload = (await response.json()) as { quotes: Quote[] };
  return payload.quotes;
}

export type OrderRequest = {
  portfolioId: string;
  assetId: string;
  side: 'buy' | 'sell';
  quantity: number;
  orderType?: 'market' | 'limit' | 'stop';
  triggerPriceCents?: number;
};

export type OrderResult = {
  portfolio: PortfolioSnapshot;
  openOrders: OpenOrder[];
};

async function orderErrorMessage(response: Response): Promise<string> {
  let message = `Order fehlgeschlagen (${response.status})`;
  try {
    const body = (await response.json()) as { error?: string };
    if (body?.error) message = body.error;
  } catch {
    // keep the status-based message
  }
  return message;
}

/**
 * Places an order server-side. A market order fills at the server's own quote
 * price; a limit/stop order is queued as a server-side open order that the
 * backend engine fills when its trigger is met (even with the browser closed).
 * The returned portfolio + open orders are authoritative — the client cannot
 * fabricate prices, cash, or positions (competitive mode).
 */
export async function placeOrder(clientId: string, input: OrderRequest): Promise<OrderResult> {
  const response = await fetch('/api/orders', {
    method: 'POST',
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/json',
      'X-Koala-Client-ID': clientId
    },
    body: JSON.stringify({ orderType: 'market', ...input })
  });

  if (!response.ok) {
    throw new Error(await orderErrorMessage(response));
  }

  const payload = (await response.json()) as { portfolio: PortfolioSnapshot; openOrders?: OpenOrder[] };
  return { portfolio: payload.portfolio, openOrders: payload.openOrders ?? [] };
}

export type LeaderboardEntry = {
  rank: number;
  displayName: string;
  totalEquityCents: number;
  totalReturnBps: number;
  isYou: boolean;
};

export async function fetchLeaderboard(): Promise<LeaderboardEntry[]> {
  const response = await fetch('/api/leaderboard', { headers: { Accept: 'application/json' } });
  if (!response.ok) {
    throw new Error(`Leaderboard request failed with ${response.status}`);
  }
  const payload = (await response.json()) as { leaderboard?: LeaderboardEntry[] };
  return payload.leaderboard ?? [];
}

export type EsportsBetRequest = {
  portfolioId: string;
  matchId: string;
  teamCode: string;
  side: 'buy' | 'sell';
  contracts: number;
};

/**
 * Buys/sells "Yes" contracts on a match winner server-side. The server prices
 * the fill from its own (freshly refreshed) Polymarket odds and validates
 * against the server portfolio, so odds and payouts can't be self-reported.
 */
export async function submitEsportsBet(clientId: string, input: EsportsBetRequest): Promise<OrderResult> {
  const response = await fetch('/api/esports/bet', {
    method: 'POST',
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/json',
      'X-Koala-Client-ID': clientId
    },
    body: JSON.stringify(input)
  });
  if (!response.ok) {
    throw new Error(await orderErrorMessage(response));
  }
  const payload = (await response.json()) as { portfolio: PortfolioSnapshot; openOrders?: OpenOrder[] };
  return { portfolio: payload.portfolio, openOrders: payload.openOrders ?? [] };
}

export async function fetchOpenOrders(clientId: string, portfolioId: string): Promise<OpenOrder[]> {
  const response = await fetch(`/api/open-orders?id=${encodeURIComponent(portfolioId)}`, {
    headers: { Accept: 'application/json', 'X-Koala-Client-ID': clientId }
  });
  if (!response.ok) {
    throw new Error(`Open orders request failed with ${response.status}`);
  }
  const payload = (await response.json()) as { openOrders?: OpenOrder[] };
  return payload.openOrders ?? [];
}

export async function cancelServerOpenOrder(clientId: string, portfolioId: string, id: string): Promise<OpenOrder[]> {
  const response = await fetch(`/api/open-orders/${encodeURIComponent(id)}?id=${encodeURIComponent(portfolioId)}`, {
    method: 'DELETE',
    headers: { Accept: 'application/json', 'X-Koala-Client-ID': clientId }
  });
  if (!response.ok) {
    throw new Error(await orderErrorMessage(response));
  }
  const payload = (await response.json()) as { openOrders?: OpenOrder[] };
  return payload.openOrders ?? [];
}

export async function syncPortfolio(
  clientId: string,
  snapshot: PortfolioSnapshot
): Promise<PortfolioSnapshot> {
  const response = await fetch('/api/sync/portfolio', {
    method: 'PUT',
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/json',
      'X-Koala-Client-ID': clientId
    },
    body: JSON.stringify(snapshot)
  });

  if (!response.ok) {
    throw new Error(`Portfolio sync failed with ${response.status}`);
  }

  const payload = (await response.json()) as { portfolio: PortfolioSnapshot };
  return payload.portfolio;
}

export async function fetchSyncedPortfolio(
  clientId: string,
  portfolioId: string
): Promise<PortfolioSnapshot | null> {
  const response = await fetch(`/api/sync/portfolio?id=${encodeURIComponent(portfolioId)}`, {
    headers: {
      Accept: 'application/json',
      'X-Koala-Client-ID': clientId
    }
  });

  if (response.status === 404) {
    return null;
  }

  if (!response.ok) {
    throw new Error(`Portfolio fetch failed with ${response.status}`);
  }

  const payload = (await response.json()) as { portfolio: PortfolioSnapshot };
  return payload.portfolio;
}
