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

export type OpenOrderType = 'limit' | 'stop';

/**
 * A pending Limit or Stop order. Unlike a Market order it does NOT execute on
 * submit — it waits in the local open-orders queue and is filled by the quote
 * poll once its trigger condition is met. This is the fix for the old UX where
 * Limit/Stop were cosmetic and filled instantly.
 */
export type OpenOrder = {
  id: string;
  assetId: string;
  symbol: string;
  name: string;
  kind: AssetKind;
  side: TransactionSide;
  orderType: OpenOrderType;
  quantity: number;
  triggerPriceCents: number;
  createdAt: string;
};

/**
 * Whether a pending Limit/Stop order should fill at the given live price.
 * - Limit buy  fills when price drops to/through the limit (buy at or better).
 * - Limit sell fills when price rises to/through the limit (sell at or better).
 * - Stop buy   fires when price rises to/through the stop (breakout / short cover).
 * - Stop sell  fires when price falls to/through the stop (classic stop-loss).
 */
export function shouldTriggerOrder(order: OpenOrder, priceCents: number): boolean {
  if (!Number.isFinite(priceCents) || priceCents <= 0) return false;
  if (order.orderType === 'limit') {
    return order.side === 'buy'
      ? priceCents <= order.triggerPriceCents
      : priceCents >= order.triggerPriceCents;
  }
  return order.side === 'buy'
    ? priceCents >= order.triggerPriceCents
    : priceCents <= order.triggerPriceCents;
}

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
      throw new Error('Not enough cash for this order');
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
      throw new Error('Not enough position size for this sell');
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

/**
 * Settles a resolved event/bet position: a winning "Yes" contract pays out 100¢
 * each, a losing one expires at 0¢. Credits cash, removes the position, and
 * records the settlement as a transaction.
 */
export function resolveEventPosition(
  snapshot: PortfolioSnapshot,
  assetId: string,
  won: boolean,
  now = new Date()
): PortfolioSnapshot | null {
  const index = snapshot.positions.findIndex((position) => position.assetId === assetId);
  if (index < 0) return null;

  const position = snapshot.positions[index];
  const payoutCents = won ? 100 : 0;
  const proceedsCents = Math.round(position.quantity * payoutCents);
  const timestamp = now.toISOString();

  const transaction: Transaction = {
    id: crypto.randomUUID(),
    assetId: position.assetId,
    symbol: position.symbol,
    side: 'sell',
    quantity: position.quantity,
    priceCents: payoutCents,
    feeCents: 0,
    status: 'local',
    createdAt: timestamp
  };

  return {
    ...snapshot,
    cashCents: snapshot.cashCents + proceedsCents,
    positions: snapshot.positions.filter((_, i) => i !== index),
    transactions: [transaction, ...snapshot.transactions],
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

export type EquityPoint = {
  t: string;
  equityCents: number;
};

export type PortfolioPerformance = {
  realizedPnlCents: number;
  unrealizedPnlCents: number;
  peakEquityCents: number;
  drawdownBps: number;
  curve: EquityPoint[];
};

/**
 * Reconstructs an approximate equity curve by replaying transactions in
 * chronological order, valuing each position at the most recent traded price.
 * Anchored with the starting cash at creation and the live equity at the end.
 */
export function computePerformance(
  snapshot: PortfolioSnapshot,
  currentEquityCents: number,
  now = new Date()
): PortfolioPerformance {
  const ordered = [...snapshot.transactions].sort(
    (a, b) => new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime()
  );

  let cashCents = snapshot.startingCashCents;
  let realizedPnlCents = 0;
  const holdings = new Map<string, { quantity: number; avgCostCents: number; lastPriceCents: number }>();
  const curve: EquityPoint[] = [{ t: snapshot.createdAt, equityCents: snapshot.startingCashCents }];

  const equityAt = () => {
    let value = cashCents;
    for (const holding of holdings.values()) {
      value += Math.round(holding.quantity * holding.lastPriceCents);
    }
    return value;
  };

  for (const tx of ordered) {
    const grossCents = Math.round(tx.quantity * tx.priceCents);
    const existing = holdings.get(tx.assetId);

    if (tx.side === 'buy') {
      cashCents -= grossCents + tx.feeCents;
      if (existing) {
        const newQuantity = existing.quantity + tx.quantity;
        existing.avgCostCents = Math.round(
          (existing.quantity * existing.avgCostCents + grossCents) / newQuantity
        );
        existing.quantity = newQuantity;
        existing.lastPriceCents = tx.priceCents;
      } else {
        holdings.set(tx.assetId, {
          quantity: tx.quantity,
          avgCostCents: tx.priceCents,
          lastPriceCents: tx.priceCents
        });
      }
    } else {
      cashCents += grossCents - tx.feeCents;
      if (existing) {
        realizedPnlCents += Math.round((tx.priceCents - existing.avgCostCents) * tx.quantity) - tx.feeCents;
        existing.quantity -= tx.quantity;
        existing.lastPriceCents = tx.priceCents;
        if (existing.quantity <= 0.000_001) {
          holdings.delete(tx.assetId);
        }
      }
    }

    curve.push({ t: tx.createdAt, equityCents: equityAt() });
  }

  curve.push({ t: now.toISOString(), equityCents: currentEquityCents });

  const unrealizedPnlCents = snapshot.positions.reduce(
    (total, position) =>
      total + Math.round((position.lastPriceCents - position.averageCostCents) * position.quantity),
    0
  );

  let peakEquityCents = snapshot.startingCashCents;
  let maxDrawdownBps = 0;
  for (const point of curve) {
    peakEquityCents = Math.max(peakEquityCents, point.equityCents);
    if (peakEquityCents > 0) {
      const ddBps = Math.round(((peakEquityCents - point.equityCents) / peakEquityCents) * 10_000);
      maxDrawdownBps = Math.max(maxDrawdownBps, ddBps);
    }
  }

  return {
    realizedPnlCents,
    unrealizedPnlCents,
    peakEquityCents,
    drawdownBps: maxDrawdownBps,
    curve
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

export function formatPrice(cents: number | undefined | null) {
  if (cents === undefined || cents === null || cents <= 0) {
    return '—';
  }
  return formatMoney(cents);
}
