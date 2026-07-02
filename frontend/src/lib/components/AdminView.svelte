<script lang="ts">
  import { Link2, LogOut, RefreshCw, ShieldCheck, Trash2 } from '@lucide/svelte';
  import {
    AdminAuthError,
    adminRefreshEsports,
    deleteTeamMapping,
    fetchAdminSettings,
    fetchAdminStatus,
    fetchTeamMappings,
    updateAdminSettings,
    type AdminSettings,
    upsertTeamMapping,
    type AdminStatus,
    type EsportsMatch,
    type TeamMapping
  } from '../api';

  export let token: string | null = null;
  export let matches: EsportsMatch[] = [];
  export let onLogin: (username: string, password: string) => Promise<void>;
  export let onLogout: () => void;

  let username = 'admin';
  let password = '';
  let loginError = '';
  let loggingIn = false;

  let mappings: TeamMapping[] = [];
  let status: AdminStatus | null = null;
  let settings: AdminSettings | null = null;
  let originalCode = '';
  let polymarketCode = '';
  let error = '';
  let busy = false;
  let refreshing = false;
  let loadedFor: string | null = null;

  $: if (token && token !== loadedFor) {
    loadedFor = token;
    void loadData();
  }
  $: if (!token) loadedFor = null;

  // Matches that currently lack Polymarket odds → candidates for a mapping.
  $: noOddsMatches = matches.filter((m) => !m.hasOdds && m.team1.code !== 'TBD' && m.team2.code !== 'TBD');

  async function handleLoginSubmit() {
    loginError = '';
    loggingIn = true;
    try {
      await onLogin(username.trim(), password);
      password = '';
    } catch (e) {
      loginError = e instanceof Error ? e.message : 'Login fehlgeschlagen';
    } finally {
      loggingIn = false;
    }
  }

  async function loadData() {
    if (!token) return;
    error = '';
    try {
      [mappings, status, settings] = await Promise.all([fetchTeamMappings(token), fetchAdminStatus(token), fetchAdminSettings(token)]);
    } catch (e) {
      handleError(e);
    }
  }

  function handleError(e: unknown) {
    if (e instanceof AdminAuthError) {
      onLogout();
      return;
    }
    error = e instanceof Error ? e.message : 'Fehler';
  }

  async function saveMapping() {
    if (!token || !originalCode.trim() || !polymarketCode.trim()) return;
    busy = true;
    error = '';
    try {
      mappings = await upsertTeamMapping(token, originalCode.trim(), polymarketCode.trim());
      originalCode = '';
      polymarketCode = '';
    } catch (e) {
      handleError(e);
    } finally {
      busy = false;
    }
  }

  async function removeMapping(code: string) {
    if (!token) return;
    try {
      mappings = await deleteTeamMapping(token, code);
    } catch (e) {
      handleError(e);
    }
  }

  async function refresh() {
    if (!token) return;
    refreshing = true;
    try {
      await adminRefreshEsports(token);
      status = await fetchAdminStatus(token);
    } catch (e) {
      handleError(e);
    } finally {
      refreshing = false;
    }
  }

  async function toggleRegistration() {
    if (!token || !settings) return;
    busy = true;
    error = '';
    try {
      settings = await updateAdminSettings(token, { registrationOpen: !settings.registrationOpen });
    } catch (e) {
      handleError(e);
    } finally {
      busy = false;
    }
  }

  function prefill(code: string) {
    originalCode = code;
    polymarketCode = '';
  }
</script>

<div class="admin">
  {#if !token}
    <section class="panel login-card">
      <div class="panel-head"><div><p class="eyebrow">Admin</p><h2>Anmelden</h2></div><ShieldCheck size={18} /></div>
      <form class="login-form" on:submit|preventDefault={handleLoginSubmit}>
        <label class="field"><span>Benutzername</span><input bind:value={username} type="text" autocomplete="username" /></label>
        <label class="field"><span>Passwort</span><input bind:value={password} type="password" autocomplete="current-password" /></label>
        {#if loginError}<p class="form-error">{loginError}</p>{/if}
        <button class="primary-button" type="submit" disabled={loggingIn || !password}>{loggingIn ? 'Anmelden …' : 'Anmelden'}</button>
        <p class="hint">Admin wird einmalig aus <code>ADMIN_USERNAME</code>/<code>ADMIN_PASSWORD</code> geseedet.</p>
      </form>
    </section>
  {:else}
    <section class="panel">
      <div class="panel-head">
        <div><p class="eyebrow">Admin</p><h2>Status & Cache</h2></div>
        <div class="head-actions">
          <button class="ghost-btn" type="button" disabled={refreshing} on:click={refresh}><RefreshCw size={15} /> {refreshing ? 'Aktualisiere …' : 'Force-Refresh'}</button>
          <button class="ghost-btn" type="button" on:click={onLogout}><LogOut size={15} /> Logout</button>
        </div>
      </div>
      {#if status}
        <div class="status-grid">
          <div><span>Schedule</span><strong>{status.esports.scheduleCached ? `${status.esports.scheduleAgeSeconds}s alt` : 'leer'}</strong></div>
          <div><span>Matches</span><strong>{status.esports.matchCount}</strong></div>
          <div><span>Mit Quote</span><strong>{status.esports.matchesWithOdds}</strong></div>
          <div><span>Ergebnisse</span><strong>{status.esports.resultsCount}</strong></div>
          <div><span>Teams</span><strong>{status.esports.teamCount}</strong></div>
          <div><span>Marktdaten</span><strong>{status.marketDataSource}</strong></div>
        </div>
      {/if}
      {#if settings}
        <div class="settings-row">
          <span>Registrierung</span>
          <button class:active={settings.registrationOpen} type="button" disabled={busy} on:click={toggleRegistration}>
            {settings.registrationOpen ? 'Offen' : 'Geschlossen'}
          </button>
        </div>
      {/if}
    </section>

    <section class="panel">
      <div class="panel-head"><div><p class="eyebrow">Polymarket</p><h2>Team-Mappings</h2></div><Link2 size={18} /></div>
      <p class="hint">Polymarket nutzt teils andere Kürzel als lolesports. Hier lolesports-Code → Polymarket-Code zuordnen (z.B. EINS → ES1).</p>

      <form class="mapping-form" on:submit|preventDefault={saveMapping}>
        <input bind:value={originalCode} type="text" placeholder="lolesports-Code (EINS)" />
        <span class="arrow">→</span>
        <input bind:value={polymarketCode} type="text" placeholder="Polymarket-Code (ES1)" />
        <button class="primary-button" type="submit" disabled={busy || !originalCode.trim() || !polymarketCode.trim()}>Speichern</button>
      </form>
      {#if error}<p class="form-error">{error}</p>{/if}

      <div class="mapping-list">
        {#if mappings.length === 0}
          <p class="empty-state">Noch keine Mappings.</p>
        {:else}
          {#each mappings as m (m.originalCode)}
            <div class="mapping-row">
              <strong>{m.originalCode}</strong><span class="arrow">→</span><strong>{m.polymarketCode}</strong>
              <button class="del" type="button" aria-label="Löschen" on:click={() => removeMapping(m.originalCode)}><Trash2 size={15} /></button>
            </div>
          {/each}
        {/if}
      </div>
    </section>

    <section class="panel">
      <div class="panel-head"><div><p class="eyebrow">Diagnose</p><h2>Matches ohne Quote</h2></div></div>
      <p class="hint">Diese anstehenden Matches haben keine Polymarket-Quote — oft ein fehlendes Mapping. Klick legt ein Mapping an.</p>
      <div class="noodds-list">
        {#if noOddsMatches.length === 0}
          <p class="empty-state">Alle sichtbaren Matches haben eine Quote.</p>
        {:else}
          {#each noOddsMatches.slice(0, 30) as m (m.id)}
            <div class="noodds-row">
              <span class="lg">{m.league}</span>
              <button type="button" on:click={() => prefill(m.team1.code)}>{m.team1.code}</button>
              <span class="vs">vs</span>
              <button type="button" on:click={() => prefill(m.team2.code)}>{m.team2.code}</button>
            </div>
          {/each}
        {/if}
      </div>
    </section>
  {/if}
</div>

<style>
  .admin {
    display: grid;
    gap: 0.75rem;
    align-content: start;
    max-width: 52rem;
    margin: 0 auto;
    width: 100%;
  }

  .login-card {
    max-width: 26rem;
    margin: 2rem auto;
    width: 100%;
  }

  .login-form,
  .field {
    display: grid;
    gap: 0.5rem;
  }

  .login-form {
    gap: 0.8rem;
  }

  .hint {
    margin: 0;
    color: var(--muted);
    font-size: 0.82rem;
  }

  .hint code,
  code {
    padding: 0.05rem 0.3rem;
    border-radius: 4px;
    background: var(--bg-2);
    font-size: 0.85em;
  }

  .head-actions {
    display: flex;
    gap: 0.4rem;
  }

  .ghost-btn {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    min-height: 2.1rem;
    padding: 0 0.75rem;
    border: 1px solid var(--line-2);
    border-radius: 6px;
    color: var(--text);
    font-size: 0.82rem;
    background: var(--panel-3);
    transition: 120ms ease;
  }

  .ghost-btn:hover:not(:disabled) {
    border-color: var(--line-strong);
  }

  .status-grid {
    display: grid;
    grid-template-columns: repeat(6, minmax(0, 1fr));
    gap: 0.5rem;
  }

  .settings-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    margin-top: 0.75rem;
    padding-top: 0.75rem;
    border-top: 1px solid var(--line);
    color: var(--muted);
    font-size: 0.86rem;
  }

  .settings-row button {
    min-height: 2rem;
    padding: 0 0.8rem;
    border: 1px solid var(--line-2);
    border-radius: 999px;
    color: var(--text);
    background: var(--panel-3);
  }

  .settings-row button.active {
    color: var(--green);
    border-color: var(--green-soft);
    background: var(--green-soft);
  }

  .status-grid div {
    display: grid;
    gap: 0.15rem;
    padding: 0.6rem 0.7rem;
    border: 1px solid var(--line);
    border-radius: var(--r-sm);
    background: var(--bg-2);
  }

  .status-grid span {
    color: var(--muted);
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .status-grid strong {
    font-size: 1rem;
  }

  .mapping-form {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
    margin: 0.75rem 0;
  }

  .mapping-form input {
    flex: 1 1 9rem;
    min-height: 2.5rem;
    padding: 0 0.7rem;
    border: 1px solid var(--line);
    border-radius: var(--r-sm);
    color: var(--text);
    background: var(--bg-2);
    outline: none;
    text-transform: uppercase;
  }

  .mapping-form .primary-button {
    flex: 0 0 auto;
  }

  .arrow {
    color: var(--muted);
  }

  .mapping-list {
    display: grid;
    gap: 0.3rem;
  }

  .mapping-row {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.5rem 0.7rem;
    border: 1px solid var(--line);
    border-radius: var(--r-sm);
    background: var(--bg-2);
  }

  .mapping-row .del {
    margin-left: auto;
    display: grid;
    place-items: center;
    width: 2rem;
    height: 2rem;
    border: 1px solid var(--line);
    border-radius: 6px;
    color: var(--muted);
    background: transparent;
  }

  .mapping-row .del:hover {
    color: var(--red);
    border-color: var(--red);
  }

  .noodds-list {
    display: grid;
    gap: 0.3rem;
    max-height: 18rem;
    overflow-y: auto;
    scrollbar-width: thin;
  }

  .noodds-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.4rem 0.6rem;
    border: 1px solid var(--line);
    border-radius: var(--r-sm);
    background: var(--bg-2);
    font-size: 0.85rem;
  }

  .noodds-row .lg {
    color: var(--muted);
    min-width: 9rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .noodds-row .vs {
    color: var(--muted);
  }

  .noodds-row button {
    padding: 0.2rem 0.55rem;
    border: 1px solid var(--line-2);
    border-radius: 6px;
    color: var(--text);
    background: var(--panel-3);
  }

  .noodds-row button:hover {
    border-color: var(--green);
    color: var(--green);
  }

  @media (max-width: 640px) {
    .status-grid {
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }
  }
</style>
