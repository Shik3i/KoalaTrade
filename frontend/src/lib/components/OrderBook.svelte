<script lang="ts">
  import { formatMoney } from '../portfolio';

  export let priceCents = 0;
  export let symbol = '';
  export let levels = 7;

  type Level = { priceCents: number; size: number; depthPct: number };

  // Deterministic pseudo-random in [0,1) seeded by an integer, so the simulated
  // book is stable for a given price but shifts as the quote moves.
  function rand(seed: number) {
    const x = Math.sin(seed * 12.9898) * 43758.5453;
    return x - Math.floor(x);
  }

  function buildSide(seedBase: number, sign: number, tick: number, half: number) {
    const rows: Level[] = [];
    let cumulative = 0;
    const sizes: number[] = [];
    for (let i = 0; i < levels; i++) {
      const size = (0.4 + rand(seedBase + i) * 2.6) * (1 - i / (levels + 3));
      sizes.push(size);
      cumulative += size;
    }
    let running = 0;
    for (let i = 0; i < levels; i++) {
      running += sizes[i];
      rows.push({
        priceCents: Math.max(1, Math.round(priceCents + sign * (half + i * tick))),
        size: sizes[i],
        depthPct: Math.round((running / cumulative) * 100)
      });
    }
    return rows;
  }

  $: tick = Math.max(1, Math.round(priceCents * 0.0002));
  $: half = Math.max(1, Math.round(priceCents * 0.0002));
  $: seed = Math.floor(priceCents / Math.max(1, tick));
  $: asks = buildSide(seed + 101, 1, tick, half).reverse();
  $: bids = buildSide(seed + 977, -1, tick, half);
  $: spread = asks.length && bids.length ? asks[asks.length - 1].priceCents - bids[0].priceCents : 0;
  $: spreadBps = priceCents > 0 ? ((spread / priceCents) * 10_000).toFixed(1) : '0.0';
</script>

<div class="book">
  <div class="book-head"><span>Preis</span><span>Größe ({symbol})</span></div>

  <div class="book-side ask">
    {#each asks as row}
      <div class="row" style={`--depth:${row.depthPct}%`}>
        <span class="px">{formatMoney(row.priceCents)}</span>
        <strong>{row.size.toFixed(3)}</strong>
      </div>
    {/each}
  </div>

  <div class="book-mid">
    <strong>{formatMoney(priceCents)}</strong>
    <span>Spread {formatMoney(spread)} · {spreadBps} bps</span>
  </div>

  <div class="book-side bid">
    {#each bids as row}
      <div class="row" style={`--depth:${row.depthPct}%`}>
        <span class="px">{formatMoney(row.priceCents)}</span>
        <strong>{row.size.toFixed(3)}</strong>
      </div>
    {/each}
  </div>
</div>

<style>
  .book {
    display: grid;
    gap: 0.5rem;
  }

  .book-head {
    display: flex;
    justify-content: space-between;
    color: var(--muted);
    font-size: 0.72rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .book-side {
    display: grid;
    gap: 0.18rem;
  }

  .row {
    position: relative;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    min-height: 1.9rem;
    padding: 0 0.55rem;
    overflow: hidden;
    border-radius: 5px;
    font-variant-numeric: tabular-nums;
  }

  .row::before {
    content: '';
    position: absolute;
    inset: 0 0 0 auto;
    width: var(--depth);
    opacity: 0.16;
  }

  .ask .row::before { background: var(--red); }
  .bid .row::before { background: var(--green); }
  .ask .px { color: var(--red); }
  .bid .px { color: var(--green); }

  .row .px,
  .row strong {
    position: relative;
    font-size: 0.82rem;
  }

  .book-mid {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 0.6rem;
    padding: 0.4rem 0.55rem;
    border-top: 1px solid var(--line);
    border-bottom: 1px solid var(--line);
  }

  .book-mid strong {
    font-size: 1.05rem;
    font-variant-numeric: tabular-nums;
  }

  .book-mid span {
    color: var(--muted);
    font-size: 0.72rem;
  }
</style>
