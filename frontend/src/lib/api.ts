import type { AssetKind } from './portfolio';

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
