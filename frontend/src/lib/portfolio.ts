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

export function formatMoney(cents: number) {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD'
  }).format(cents / 100);
}

export function formatPercentFromBps(bps: number) {
  return `${(bps / 100).toFixed(2)}%`;
}
