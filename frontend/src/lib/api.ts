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
): Promise<PortfolioSnapshot> {
  const response = await fetch(`/api/sync/portfolio?id=${encodeURIComponent(portfolioId)}`, {
    headers: {
      Accept: 'application/json',
      'X-Koala-Client-ID': clientId
    }
  });

  if (!response.ok) {
    throw new Error(`Portfolio fetch failed with ${response.status}`);
  }

  const payload = (await response.json()) as { portfolio: PortfolioSnapshot };
  return payload.portfolio;
}
