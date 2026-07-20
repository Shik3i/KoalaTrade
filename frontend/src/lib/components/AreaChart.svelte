<script lang="ts" context="module">
  let uid = 0;
</script>

<script lang="ts">
  export let series: number[] = [];
  export let labels: string[] = [];
  export let overlay: number[] | null = null;
  export let height = 280;
  export let loading = false;
  export let formatValue: (value: number) => string = (value) => `${value}`;
  export let formatLabel: (label: string) => string = (label) => label;
  export let accent: 'auto' | 'up' | 'down' = 'auto';
  import { t } from '../i18n';

  const gradientId = `area-grad-${++uid}`;
  const padX = 8;
  const padTop = 14;
  const padBottom = 18;

  let width = 720;
  let hoverIndex: number | null = null;

  $: tone =
    accent !== 'auto'
      ? accent
      : series.length > 1 && series[series.length - 1] >= series[0]
        ? 'up'
        : 'down';
  $: strokeColor = tone === 'up' ? 'var(--green)' : 'var(--red)';

  $: minValue = series.length ? Math.min(...series, ...(overlay ?? [])) : 0;
  $: maxValue = series.length ? Math.max(...series, ...(overlay ?? [])) : 1;
  $: span = maxValue - minValue || Math.max(1, Math.abs(maxValue) * 0.02);
  $: padded = { lo: minValue - span * 0.12, hi: maxValue + span * 0.12 };

  const plotX = (index: number, total: number) =>
    total <= 1 ? width / 2 : padX + (index / (total - 1)) * (width - padX * 2);
  const plotY = (value: number) =>
    padTop + (1 - (value - padded.lo) / (padded.hi - padded.lo || 1)) * (height - padTop - padBottom);

  $: linePath = buildPath(series);
  $: areaPath =
    series.length > 1
      ? `${linePath} L ${plotX(series.length - 1, series.length).toFixed(2)} ${(height - padBottom).toFixed(2)} L ${plotX(0, series.length).toFixed(2)} ${(height - padBottom).toFixed(2)} Z`
      : '';
  $: overlayPath = overlay && overlay.length > 1 ? buildPath(overlay) : '';

  function buildPath(points: number[]) {
    if (points.length === 0) return '';
    return points
      .map((value, index) => `${index === 0 ? 'M' : 'L'} ${plotX(index, points.length).toFixed(2)} ${plotY(value).toFixed(2)}`)
      .join(' ');
  }

  function handleMove(event: PointerEvent) {
    if (series.length < 2) return;
    const rect = (event.currentTarget as SVGElement).getBoundingClientRect();
    const ratio = Math.min(1, Math.max(0, (event.clientX - rect.left - padX) / (rect.width - padX * 2)));
    hoverIndex = Math.round(ratio * (series.length - 1));
  }

  function clearHover() {
    hoverIndex = null;
  }

  $: hoverX = hoverIndex !== null ? plotX(hoverIndex, series.length) : 0;
  $: hoverY = hoverIndex !== null ? plotY(series[hoverIndex]) : 0;
  $: tooltipLeft = `${Math.min(Math.max((hoverX / width) * 100, 12), 88)}%`;
</script>

<div class="chart" style={`height:${height}px`} bind:clientWidth={width}>
  {#if loading}
    <div class="chart-skeleton" aria-hidden="true"></div>
  {:else if series.length < 2}
    <p class="chart-empty">{$t('areaChart.empty')}</p>
  {:else}
    <svg
      viewBox={`0 0 ${width} ${height}`}
      width="100%"
      height={height}
      role="img"
      on:pointermove={handleMove}
      on:pointerleave={clearHover}
    >
      <defs>
        <linearGradient id={gradientId} x1="0" x2="0" y1="0" y2="1">
          <stop offset="0%" stop-color={strokeColor} stop-opacity="0.22" />
          <stop offset="50%" stop-color={strokeColor} stop-opacity="0.04" />
          <stop offset="100%" stop-color={strokeColor} stop-opacity="0" />
        </linearGradient>
      </defs>

      {#each [0.25, 0.5, 0.75] as line}
        <line class="grid" x1={padX} x2={width - padX} y1={padTop + line * (height - padTop - padBottom)} y2={padTop + line * (height - padTop - padBottom)} />
      {/each}

      <path d={areaPath} fill={`url(#${gradientId})`} />
      {#if overlayPath}
        <path class="overlay" d={overlayPath} />
      {/if}
      <path class="line" d={linePath} stroke={strokeColor} />

      {#if hoverIndex !== null}
        <line class="crosshair" x1={hoverX} x2={hoverX} y1={padTop} y2={height - padBottom} />
        <circle class="cursor" cx={hoverX} cy={hoverY} r="4.5" stroke={strokeColor} />
      {/if}
    </svg>

    {#if hoverIndex !== null}
      <div class="tooltip" style={`left:${tooltipLeft}`}>
        <strong>{formatValue(series[hoverIndex])}</strong>
        {#if labels[hoverIndex]}<span>{formatLabel(labels[hoverIndex])}</span>{/if}
      </div>
    {/if}
  {/if}
</div>

<style>
  .chart {
    position: relative;
    width: 100%;
  }

  svg {
    display: block;
    overflow: visible;
    touch-action: pan-y pinch-zoom;
  }

  .grid {
    stroke: var(--line);
    stroke-width: 1;
    stroke-dasharray: 2 6;
    opacity: 0.6;
  }

  .line {
    fill: none;
    stroke-width: 2;
    stroke-linecap: round;
    stroke-linejoin: round;
  }

  .overlay {
    fill: none;
    stroke: var(--amber);
    stroke-width: 1.4;
    stroke-dasharray: 4 4;
    opacity: 0.75;
  }

  .crosshair {
    stroke: var(--line-strong);
    stroke-width: 1;
    stroke-dasharray: 3 4;
  }

  .cursor {
    fill: var(--bg);
    stroke-width: 2;
  }

  .tooltip {
    position: absolute;
    top: 0.35rem;
    transform: translateX(-50%);
    display: grid;
    gap: 0.05rem;
    padding: 0.35rem 0.6rem;
    border: 1px solid var(--line-strong);
    border-radius: 8px;
    background: rgba(8, 10, 12, 0.92);
    box-shadow: var(--shadow);
    pointer-events: none;
    white-space: nowrap;
    transition: left 120ms cubic-bezier(0.25, 1, 0.5, 1);
  }

  .tooltip strong {
    font-size: 0.92rem;
  }

  .tooltip span {
    color: var(--muted);
    font-size: 0.72rem;
  }

  .chart-skeleton {
    width: 100%;
    height: 100%;
    border-radius: var(--r);
    background: linear-gradient(100deg, rgba(255, 255, 255, 0.03) 30%, rgba(255, 255, 255, 0.08) 50%, rgba(255, 255, 255, 0.03) 70%);
    background-size: 200% 100%;
    animation: shimmer 1.3s infinite;
  }

  .chart-empty {
    display: grid;
    place-items: center;
    height: 100%;
    margin: 0;
    color: var(--muted);
  }

  @keyframes shimmer {
    to {
      background-position: -200% 0;
    }
  }
</style>
