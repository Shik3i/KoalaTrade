<script lang="ts">
  export let values: number[] = [];
  export let width = 88;
  export let height = 30;
  export let tone: 'up' | 'down' | 'flat' = 'flat';

  const color = { up: 'var(--green)', down: 'var(--red)', flat: 'var(--cyan)' };

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
</script>

{#if values.length > 1}
  <svg class="spark" {width} {height} viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none" aria-hidden="true">
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
