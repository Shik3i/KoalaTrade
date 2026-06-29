export const PORTFOLIO_ID = 'local-default';
export const PORTFOLIO_SCHEMA_VERSION = 1;

export type AssetKind = 'stock' | 'etf' | 'crypto' | 'commodity' | 'event';
export type TransactionStatus = 'local' | 'synced';
export type TransactionSide = 'buy' | 'sell';

export type Position = {
  assetId: string;
  symbol: string;
  name: string;
  kind: AssetKind;
  quantity: number;
  averageCostCents: number;
  lastPriceCents: number;
  updatedAt: string;
};

export type Transaction = {
  id: string;
  assetId: string;
  symbol: string;
  side: TransactionSide;
  quantity: number;
  priceCents: number;
  feeCents: number;
  status: TransactionStatus;
  createdAt: string;
};

export type PortfolioSnapshot = {
  id: string;
  schemaVersion: number;
  startingCashCents: number;
  cashCents: number;
  positions: Position[];
  transactions: Transaction[];
  createdAt: string;
  updatedAt: string;
};

export type PortfolioSummary = {
  cashCents: number;
  positionsValueCents: number;
  totalEquityCents: number;
  totalReturnCents: number;
  totalReturnBps: number;
  openPositions: number;
  localTransactionCount: number;
};

export type TradeInput = {
  id: string;
  assetId: string;
  symbol: string;
  name: string;
  kind: AssetKind;
  side: TransactionSide;
  quantity: number;
  priceCents: number;
  feeCents?: number;
};

export type PriceUpdate = {
  assetId: string;
  priceCents: number;
  updatedAt?: string;
};

export function createInitialPortfolio(startingCashCents: number, now = new Date()): PortfolioSnapshot {
  const timestamp = now.toISOString();

  return {
    id: PORTFOLIO_ID,
    schemaVersion: PORTFOLIO_SCHEMA_VERSION,
    startingCashCents,
    cashCents: startingCashCents,
    positions: [],
    transactions: [],
    createdAt: timestamp,
    updatedAt: timestamp
  };
}

export function applyTrade(
  snapshot: PortfolioSnapshot,
  input: TradeInput,
  now = new Date()
): PortfolioSnapshot {
  if (!Number.isFinite(input.quantity) || input.quantity <= 0) {
    throw new Error('Quantity must be greater than zero');
  }
  if (!Number.isFinite(input.priceCents) || input.priceCents <= 0) {
    throw new Error('Price must be greater than zero');
  }

  const feeCents = input.feeCents ?? 0;
  const grossCents = Math.round(input.quantity * input.priceCents);
  const timestamp = now.toISOString();
  const positions = snapshot.positions.map((position) => ({ ...position }));
  const transactions = [...snapshot.transactions];
  const existingIndex = positions.findIndex((position) => position.assetId === input.assetId);
  const existing = existingIndex >= 0 ? positions[existingIndex] : undefined;
  let cashCents = snapshot.cashCents;

  if (input.side === 'buy') {
    const totalCostCents = grossCents + feeCents;
    if (totalCostCents > cashCents) {
      throw new Error('Not enough cash for this simulated order');
    }

    cashCents -= totalCostCents;
    if (existing) {
      const oldCostCents = existing.quantity * existing.averageCostCents;
      const newQuantity = existing.quantity + input.quantity;
      positions[existingIndex] = {
        ...existing,
        quantity: newQuantity,
        averageCostCents: Math.round((oldCostCents + grossCents) / newQuantity),
        lastPriceCents: input.priceCents,
        updatedAt: timestamp
      };
    } else {
      positions.push({
        assetId: input.assetId,
        symbol: input.symbol,
        name: input.name,
        kind: input.kind,
        quantity: input.quantity,
        averageCostCents: input.priceCents,
        lastPriceCents: input.priceCents,
        updatedAt: timestamp
      });
    }
  } else {
    if (!existing || existing.quantity < input.quantity) {
      throw new Error('Not enough position size for this simulated sell');
    }

    cashCents += grossCents - feeCents;
    const newQuantity = existing.quantity - input.quantity;
    if (newQuantity <= 0.000_001) {
      positions.splice(existingIndex, 1);
    } else {
      positions[existingIndex] = {
        ...existing,
        quantity: newQuantity,
        lastPriceCents: input.priceCents,
        updatedAt: timestamp
      };
    }
  }

  transactions.unshift({
    id: input.id,
    assetId: input.assetId,
    symbol: input.symbol,
    side: input.side,
    quantity: input.quantity,
    priceCents: input.priceCents,
    feeCents,
    status: 'local',
    createdAt: timestamp
  });

  return {
    ...snapshot,
    cashCents,
    positions,
    transactions,
    updatedAt: timestamp
  };
}

export function summarizePortfolio(snapshot: PortfolioSnapshot): PortfolioSummary {
  const positionsValueCents = snapshot.positions.reduce(
    (total, position) => total + Math.round(position.quantity * position.lastPriceCents),
    0
  );
  const totalEquityCents = snapshot.cashCents + positionsValueCents;
  const totalReturnCents = totalEquityCents - snapshot.startingCashCents;
  const totalReturnBps =
    snapshot.startingCashCents > 0
      ? Math.round((totalReturnCents / snapshot.startingCashCents) * 10_000)
      : 0;

  return {
    cashCents: snapshot.cashCents,
    positionsValueCents,
    totalEquityCents,
    totalReturnCents,
    totalReturnBps,
    openPositions: snapshot.positions.length,
    localTransactionCount: snapshot.transactions.filter((transaction) => transaction.status === 'local')
      .length
  };
}

export function markPositionsToMarket(
  snapshot: PortfolioSnapshot,
  updates: PriceUpdate[],
  now = new Date()
): PortfolioSnapshot {
  if (snapshot.positions.length === 0 || updates.length === 0) {
    return snapshot;
  }

  const byAsset = new Map(updates.map((update) => [update.assetId, update]));
  let changed = false;
  const timestamp = now.toISOString();
  const positions = snapshot.positions.map((position) => {
    const update = byAsset.get(position.assetId);
    if (!update || update.priceCents <= 0 || update.priceCents === position.lastPriceCents) {
      return position;
    }
    changed = true;
    return {
      ...position,
      lastPriceCents: update.priceCents,
      updatedAt: update.updatedAt ?? timestamp
    };
  });

  return changed ? { ...snapshot, positions, updatedAt: timestamp } : snapshot;
}

export function formatMoney(cents: number) {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD'
  }).format(cents / 100);
}

export function formatPercentFromBps(bps: number) {
  return `${(bps / 100).toFixed(2)}%`;
}
