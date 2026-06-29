import type { AssetKind, PortfolioSnapshot } from './portfolio';

export type PublicConfig = {
  appName: string;
  environment: string;
  startingCashCents: number;
  marketDataSource: string;
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

export async function adminLogin(username: string, password: string): Promise<{ token: string; expiresAt: string }> {
  const response = await fetch('/api/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
    body: JSON.stringify({ username, password })
  });
  if (response.status === 401) throw new Error('Falsche Zugangsdaten');
  if (!response.ok) throw new Error(`Login fehlgeschlagen (${response.status})`);
  return (await response.json()) as { token: string; expiresAt: string };
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

export async function fetchAdminStatus(token: string): Promise<AdminStatus> {
  const response = await fetch('/api/admin/status', { headers: adminHeaders(token) });
  return adminJson<AdminStatus>(response);
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
