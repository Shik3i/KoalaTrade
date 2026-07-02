<script lang="ts">
  export let values: number[] = [];
  export let width = 88;
  export let height = 30;
  export let tone: 'up' | 'down' | 'flat' = 'flat';

  const color = { up: 'var(--green)', down: 'var(--red)', flat: 'var(--cyan)' };

  const uid = Math.random().toString(36).substring(2, 8);
  $: gradientId = `spark-grad-${uid}`;

  $: min = values.length ? Math.min(...values) : 0;
  $: max = values.length ? Math.max(...values) : 1;
  $: span = max - min || 1;
  $: path = values
    .map((value, index) => {
      const x = values.length <= 1 ? width / 2 : (index / (values.length - 1)) * width;
      const y = height - 2 - ((value - min) / span) * (height - 4);
      return `${index === 0 ? 'M' : 'L'} ${x.toFixed(1)} ${y.toFixed(1)}`;
    })
    .join(' ');

  $: areaPath = values.length > 1
    ? `${path} L ${width.toFixed(1)} ${(height + 2).toFixed(1)} L 0.0 ${(height + 2).toFixed(1)} Z`
    : '';
</script>

{#if values.length > 1}
  <svg class="spark" {width} {height} viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none" aria-hidden="true">
    <defs>
      <linearGradient id={gradientId} x1="0" x2="0" y1="0" y2="1">
        <stop offset="0%" stop-color={color[tone]} stop-opacity="0.15" />
        <stop offset="100%" stop-color={color[tone]} stop-opacity="0" />
      </linearGradient>
    </defs>
    <path d={areaPath} fill={`url(#${gradientId})`} />
    <path d={path} fill="none" stroke={color[tone]} stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" />
  </svg>
{:else}
  <span class="spark-empty" aria-hidden="true"></span>
{/if}

<style>
  .spark {
    display: block;
    overflow: visible;
  }

  .spark-empty {
    display: block;
    width: 100%;
    height: 2px;
    border-radius: 99px;
    background: var(--line);
  }
</style>
