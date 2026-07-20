<script lang="ts">
  import { Info } from '@lucide/svelte';
  import { t } from '../i18n';

  export let text: string;
  export let label: string | undefined = undefined;
  $: resolvedLabel = label ?? $t('common.explanation');
  // Preferred side; flips automatically near screen edges via CSS is not possible,
  // so callers can pass 'top' | 'bottom' when the default would clip.
  export let placement: 'top' | 'bottom' = 'top';
  // Horizontal anchoring of the bubble. Use 'right' for tooltips near the right
  // screen edge so the bubble doesn't overflow the viewport.
  export let align: 'center' | 'left' | 'right' = 'center';
</script>

<span class="infotip" class:bottom={placement === 'bottom'} class:align-left={align === 'left'} class:align-right={align === 'right'} tabindex="0" role="button" aria-label={resolvedLabel}>
  <Info size={13} aria-hidden="true" />
  <span class="infotip-bubble" role="tooltip">{text}</span>
</span>

<style>
  .infotip {
    position: relative;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    margin-left: 0.28rem;
    color: var(--muted);
    cursor: help;
    vertical-align: middle;
    outline: none;
  }

  .infotip:hover,
  .infotip:focus-visible {
    color: var(--text);
  }

  .infotip-bubble {
    position: absolute;
    bottom: calc(100% + 8px);
    left: 50%;
    transform: translateX(-50%) translateY(4px);
    z-index: 40;
    width: max-content;
    max-width: 15rem;
    padding: 0.45rem 0.6rem;
    border: 1px solid var(--line-strong);
    border-radius: 8px;
    background: rgba(8, 10, 12, 0.97);
    box-shadow: var(--shadow);
    color: var(--text);
    font-size: 0.74rem;
    font-weight: 400;
    line-height: 1.4;
    text-align: left;
    text-transform: none;
    letter-spacing: normal;
    white-space: normal;
    opacity: 0;
    pointer-events: none;
    transition: opacity 120ms ease, transform 120ms ease;
  }

  .infotip.bottom .infotip-bubble {
    bottom: auto;
    top: calc(100% + 8px);
    transform: translateX(-50%) translateY(-4px);
  }

  /* Right-anchored: bubble's right edge sits at the icon, growing leftwards. */
  .infotip.align-right .infotip-bubble {
    left: auto;
    right: 0;
    transform: translateX(0) translateY(4px);
  }
  .infotip.align-left .infotip-bubble {
    left: 0;
    right: auto;
    transform: translateX(0) translateY(4px);
  }

  .infotip:hover .infotip-bubble,
  .infotip:focus-visible .infotip-bubble,
  .infotip:focus-within .infotip-bubble {
    opacity: 1;
    transform: translateX(-50%) translateY(0);
  }

  .infotip.align-right:hover .infotip-bubble,
  .infotip.align-right:focus-visible .infotip-bubble,
  .infotip.align-right:focus-within .infotip-bubble,
  .infotip.align-left:hover .infotip-bubble,
  .infotip.align-left:focus-visible .infotip-bubble,
  .infotip.align-left:focus-within .infotip-bubble {
    transform: translateX(0) translateY(0);
  }
</style>
