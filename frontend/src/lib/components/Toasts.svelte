<script lang="ts">
  import { CheckCircle2, Info, X, XCircle } from '@lucide/svelte';
  import { dismissToast, toasts } from '../toast';
  import { t } from '../i18n';

  const icons = { success: CheckCircle2, error: XCircle, info: Info };
</script>

<div class="toast-stack" aria-live="polite">
  {#each $toasts as toast (toast.id)}
    <div class={`toast ${toast.tone}`} role="status">
      <svelte:component this={icons[toast.tone]} size={18} />
      <div class="toast-body">
        <strong>{toast.title}</strong>
        {#if toast.detail}<span>{toast.detail}</span>{/if}
      </div>
      <button type="button" aria-label={$t('common.close')} on:click={() => dismissToast(toast.id)}>
        <X size={15} />
      </button>
    </div>
  {/each}
</div>

<style>
  .toast-stack {
    position: fixed;
    right: 1rem;
    bottom: 1rem;
    z-index: 60;
    display: grid;
    gap: 0.6rem;
    width: min(22rem, calc(100vw - 2rem));
  }

  .toast {
    display: grid;
    grid-template-columns: auto 1fr auto;
    align-items: start;
    gap: 0.7rem;
    padding: 0.8rem 0.9rem;
    border: 1px solid var(--line-strong);
    border-left-width: 3px;
    border-radius: 10px;
    background: rgba(16, 20, 24, 0.97);
    box-shadow: var(--shadow);
    animation: toast-in 220ms ease;
  }

  .toast.success { border-left-color: var(--green); color: var(--green); }
  .toast.error { border-left-color: var(--red); color: var(--red); }
  .toast.info { border-left-color: var(--cyan); color: var(--cyan); }

  .toast-body {
    display: grid;
    gap: 0.12rem;
    min-width: 0;
  }

  .toast-body strong {
    color: var(--text);
    font-size: 0.9rem;
  }

  .toast-body span {
    color: var(--muted);
    font-size: 0.78rem;
  }

  .toast button {
    display: grid;
    place-items: center;
    width: 1.5rem;
    height: 1.5rem;
    border: 0;
    border-radius: 6px;
    color: var(--muted);
    background: transparent;
  }

  .toast button:hover {
    color: var(--text);
    background: rgba(255, 255, 255, 0.06);
  }

  @keyframes toast-in {
    from { opacity: 0; transform: translateY(8px); }
    to { opacity: 1; transform: translateY(0); }
  }
</style>
