import type { Market } from './api';

export type PriceFreshness = 'fresh' | 'closed' | 'stale';

export const FRESH_PRICE_MS = 2 * 60 * 60 * 1000;
export const MARKET_CLOSED_MAX_MS = 4 * 24 * 60 * 60 * 1000;

export function priceFreshness(market: Market): PriceFreshness {
  if (!market || market.priceCents <= 0 || !market.updatedAt) return 'fresh';
  const t = new Date(market.updatedAt).getTime();
  if (!Number.isFinite(t)) return 'fresh';
  const age = Date.now() - t;
  if (age <= FRESH_PRICE_MS) return 'fresh';
  if (market.kind === 'crypto') return 'stale';
  return age > MARKET_CLOSED_MAX_MS ? 'stale' : 'closed';
}

export function isStalePrice(market: Market): boolean {
  return priceFreshness(market) === 'stale';
}

export function marketTone(changeBps: number) {
  if (changeBps > 0) return 'up';
  if (changeBps < 0) return 'down';
  return 'flat';
}

export function changeColor(bps: number) {
  return bps > 0 ? 'up' : bps < 0 ? 'down' : 'flat';
}

export function simpleMovingAverage(values: number[], period: number) {
  const result: number[] = [];
  for (let i = 0; i < values.length; i++) {
    const start = Math.max(0, i - period + 1);
    const window = values.slice(start, i + 1);
    result.push(window.reduce((sum, value) => sum + value, 0) / window.length);
  }
  return result;
}
