<script lang="ts">
  import { FlaskConical, Link2, LogOut, RefreshCw, ShieldCheck, Trash2 } from '@lucide/svelte';
  import {
    AdminAuthError,
    adminRefreshEsports,
    deleteTeamMapping,
    fetchAdminSettings,
    fetchAdminStatus,
    fetchTeamMappings,
    previewTeamMapping,
    updateAdminSettings,
    type AdminSettings,
    type SlugDiagnostic,
    upsertTeamMapping,
    type AdminStatus,
    type EsportsMatch,
    type TeamMapping
  } from '../api';
  import { get } from 'svelte/store';
  import { t } from '../i18n';
  const tr = (key: string, vars?: Record<string, string | number>) => get(t)(key, vars);

  export let token: string | null = null;
  export let matches: EsportsMatch[] = [];
  export let onLogin: (username: string, password: string) => Promise<void>;
  export let onLogout: () => void;
  export let onRefreshMatches: () => Promise<void> = async () => {};

  let username = 'admin';
  let password = '';
  let loginError = '';
  let loggingIn = false;

  let mappings: TeamMapping[] = [];
  let status: AdminStatus | null = null;
  let settings: AdminSettings | null = null;
  let originalCode = '';
  let polymarketCode = '';
  let selectedMatchId = '';
  let slugDiagnostic: SlugDiagnostic | null = null;
  let slugBusy = false;
  let slugError = '';
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
  $: selectedMatch = matches.find((m) => m.id === selectedMatchId) ?? noOddsMatches[0] ?? null;
  $: mappingTeam = selectedMatch
    ? [selectedMatch.team1, selectedMatch.team2].find((team) => team.code === originalCode.trim().toUpperCase()) ?? null
    : null;

  // --- Per-team slug management -------------------------------------------
  // Show every upcoming/live match so the admin can spot, at a glance, which
  // team already has a Polymarket-code mapping in the DB and which is missing —
  // and add/update it inline right on that team's row.
  let slugFilter: 'missing' | 'all' = 'missing';
  let slugEdits: Record<string, string> = {};
  let savingCode = '';

  $: mappingByCode = new Map(mappings.map((m) => [m.originalCode.toUpperCase(), m.polymarketCode]));

  function mappedCode(code: string): string | null {
    return mappingByCode.get(code.trim().toUpperCase()) ?? null;
  }

  $: playableMatches = matches.filter((m) => m.team1.code !== 'TBD' && m.team2.code !== 'TBD');
  $: teamSlugMatches =
    slugFilter === 'all'
      ? playableMatches
      : playableMatches.filter((m) => !mappedCode(m.team1.code) || !mappedCode(m.team2.code));

  // Matches where at least one team still lacks a Polymarket-code mapping.
  $: incompleteMatchCount = playableMatches.filter((m) => !mappedCode(m.team1.code) || !mappedCode(m.team2.code)).length;

  function slugValue(code: string): string {
    const key = code.trim().toUpperCase();
    if (key in slugEdits) return slugEdits[key];
    return mappedCode(code) ?? '';
  }

  function setSlug(code: string, value: string) {
    slugEdits = { ...slugEdits, [code.trim().toUpperCase()]: value };
  }

  async function saveTeamSlug(code: string) {
    if (!token) return;
    const poly = slugValue(code).trim();
    const original = code.trim();
    if (!original || !poly) return;
    savingCode = original.toUpperCase();
    error = '';
    try {
      mappings = await upsertTeamMapping(token, original, poly);
      // Drop the local edit so the row reflects the saved mapping, then
      // re-fetch odds so a now-complete match lights up immediately.
      const { [original.toUpperCase()]: _drop, ...rest } = slugEdits;
      slugEdits = rest;
      await onRefreshMatches();
      status = await fetchAdminStatus(token);
    } catch (e) {
      handleError(e);
    } finally {
      savingCode = '';
    }
  }

  async function clearTeamSlug(code: string) {
    if (!token) return;
    try {
      mappings = await deleteTeamMapping(token, code.trim());
      const { [code.trim().toUpperCase()]: _drop, ...rest } = slugEdits;
      slugEdits = rest;
      await onRefreshMatches();
    } catch (e) {
      handleError(e);
    }
  }

  async function testTeamSlug(match: EsportsMatch, code: string) {
    if (!token) return;
    selectedMatchId = match.id;
    originalCode = code;
    polymarketCode = slugValue(code).trim();
    await testMapping(true);
  }

  async function handleLoginSubmit() {
    loginError = '';
    loggingIn = true;
    try {
      await onLogin(username.trim(), password);
      password = '';
    } catch (e) {
      loginError = e instanceof Error ? e.message : tr('admin.errLoginFailed');
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
    error = e instanceof Error ? e.message : tr('admin.errGeneric');
  }

  async function saveMapping() {
    if (!token || !originalCode.trim() || !polymarketCode.trim()) return;
    busy = true;
    error = '';
    try {
      mappings = await upsertTeamMapping(token, originalCode.trim(), polymarketCode.trim());
      originalCode = '';
      polymarketCode = '';
      slugDiagnostic = null;
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

  async function testMapping(liveTest = true) {
    if (!token || !selectedMatch || !originalCode.trim()) return;
    slugBusy = true;
    slugError = '';
    try {
      slugDiagnostic = await previewTeamMapping(token, {
        matchId: selectedMatch.id,
        originalCode: originalCode.trim(),
        polymarketCode: polymarketCode.trim(),
        liveTest
      });
    } catch (e) {
      slugError = e instanceof Error ? e.message : tr('admin.errSlugTest');
    } finally {
      slugBusy = false;
    }
  }

</script>

<div class="admin">
  {#if !token}
    <section class="panel login-card">
      <div class="panel-head"><div><p class="eyebrow">{$t('admin.login')}</p><h2>{$t('admin.loginTitle')}</h2></div><ShieldCheck size={18} /></div>
      <form class="login-form" on:submit|preventDefault={handleLoginSubmit}>
        <label class="field" title={$t('admin.usernameTitle')}><span>{$t('admin.username')}</span><input bind:value={username} type="text" autocomplete="username" title={$t('admin.usernameInputTitle')} /></label>
        <label class="field" title={$t('admin.passwordTitle')}><span>{$t('admin.password')}</span><input bind:value={password} type="password" autocomplete="current-password" title={$t('admin.passwordInputTitle')} /></label>
        {#if loginError}<p class="form-error">{loginError}</p>{/if}
        <button class="primary-button" type="submit" title={$t('admin.loginSubmitTitle')} disabled={loggingIn || !password}>{loggingIn ? $t('admin.loginSubmitting') : $t('admin.login')}</button>
        <p class="hint">{$t('admin.seedHint')}</p>
      </form>
    </section>
  {:else}
    <section class="panel">
      <div class="panel-head">
        <div><p class="eyebrow">Admin</p><h2>{$t('admin.statusCache')}</h2></div>
        <div class="head-actions">
          <button class="ghost-btn" type="button" title={$t('admin.refreshTitle')} disabled={refreshing} on:click={refresh}><RefreshCw size={15} /> {refreshing ? $t('admin.refreshing') : $t('admin.forceRefresh')}</button>
          <button class="ghost-btn" type="button" title={$t('admin.logoutTitle')} on:click={onLogout}><LogOut size={15} /> {$t('admin.logout')}</button>
        </div>
      </div>
      {#if status}
        <div class="status-grid">
          <div><span>{$t('admin.statusSchedule')}</span><strong>{status.esports.scheduleCached ? $t('admin.secondsOld', { age: status.esports.scheduleAgeSeconds }) : $t('admin.scheduleEmpty')}</strong></div>
          <div><span>{$t('admin.statusMatches')}</span><strong>{status.esports.matchCount}</strong></div>
          <div><span>{$t('admin.statusWithOdds')}</span><strong>{status.esports.matchesWithOdds}</strong></div>
          <div><span>{$t('admin.statusResults')}</span><strong>{status.esports.resultsCount}</strong></div>
          <div><span>{$t('admin.statusTeams')}</span><strong>{status.esports.teamCount}</strong></div>
          <div><span>{$t('admin.statusMarketData')}</span><strong>{status.marketDataSource}</strong></div>
        </div>
      {/if}
      {#if settings}
        <div class="settings-row">
          <span>{$t('admin.registration')}</span>
          <button class:active={settings.registrationOpen} type="button" title={$t('admin.registrationTitle')} disabled={busy} on:click={toggleRegistration}>
            {settings.registrationOpen ? $t('admin.regOpen') : $t('admin.regClosed')}
          </button>
        </div>
      {/if}
    </section>

    <section class="panel">
      <div class="panel-head"><div><p class="eyebrow">{$t('admin.polymarket')}</p><h2>{$t('admin.teamMappings')}</h2></div><Link2 size={18} /></div>
      <p class="hint">{$t('admin.mappingHint')}</p>

      <form class="mapping-form" on:submit|preventDefault={saveMapping}>
        <input bind:value={originalCode} type="text" placeholder={$t('admin.lolesportsPlaceholder')} title={$t('admin.lolesportsTitle')} />
        <span class="arrow">→</span>
        <input bind:value={polymarketCode} type="text" placeholder={$t('admin.pmPlaceholder')} title={$t('admin.pmTitle')} />
        <button class="primary-button" type="submit" title={$t('admin.saveMappingTitle')} disabled={busy || !originalCode.trim() || !polymarketCode.trim()}>{$t('admin.saveMappingBtn')}</button>
        <button class="ghost-btn" type="button" title={$t('admin.testSlugsTitle')} disabled={slugBusy || !selectedMatch || !originalCode.trim()} on:click={() => testMapping(true)}>
          <FlaskConical size={15} /> {slugBusy ? $t('admin.testing') : $t('admin.testSlugs')}
        </button>
      </form>
      {#if error}<p class="form-error">{error}</p>{/if}
      {#if selectedMatch}
        <div class="mapping-context">
          <span>{selectedMatch.league}</span>
          <strong>{selectedMatch.team1.code}</strong><em>{selectedMatch.team1.name}</em>
          <span class="vs">{$t('admin.vs')}</span>
          <strong>{selectedMatch.team2.code}</strong><em>{selectedMatch.team2.name}</em>
          {#if mappingTeam}<span class="candidate">{$t('admin.mappingFor', { name: mappingTeam.name })}</span>{/if}
        </div>
      {/if}
      {#if slugError}<p class="form-error">{slugError}</p>{/if}
      {#if slugDiagnostic}
        <div class="slug-diagnostic">
          <div class="slug-result" class:found={slugDiagnostic.found}>
            <span>{slugDiagnostic.found ? $t('admin.slugHit') : $t('admin.slugNoHit')}</span>
            <strong>{slugDiagnostic.found ? slugDiagnostic.eventSlug : $t('admin.slugCandidates', { count: slugDiagnostic.slugs.length })}</strong>
            {#if slugDiagnostic.polymarketUrl}<a href={slugDiagnostic.polymarketUrl} target="_blank" rel="noreferrer">{$t('admin.openPolymarket')}</a>{/if}
          </div>
          <div class="slug-list">
            {#each slugDiagnostic.slugs.slice(0, 18) as slug}
              <code class:hit={slug === slugDiagnostic.eventSlug}>{slug}</code>
            {/each}
          </div>
        </div>
      {/if}

      <div class="mapping-list">
        {#if mappings.length === 0}
          <p class="empty-state">{$t('admin.noMappings')}</p>
        {:else}
          {#each mappings as m (m.originalCode)}
            <div class="mapping-row">
              <strong>{m.originalCode}</strong><span class="arrow">→</span><strong>{m.polymarketCode}</strong>
              <button class="del" type="button" aria-label={$t('admin.deleteMappingFor', { code: m.originalCode, mapped: m.polymarketCode })} title={$t('admin.deleteMappingTitle')} on:click={() => removeMapping(m.originalCode)}><Trash2 size={15} /></button>
            </div>
          {/each}
        {/if}
      </div>
    </section>

    <section class="panel">
      <div class="panel-head">
        <div><p class="eyebrow">{$t('admin.diagnosis')}</p><h2>{$t('admin.teamSlugsPerMatch')}</h2></div>
        <div class="slug-filter">
          <button class:active={slugFilter === 'missing'} type="button" title={$t('admin.filterMissingTitle')} on:click={() => (slugFilter = 'missing')}>{$t('admin.filterIncomplete', { count: incompleteMatchCount })}</button>
          <button class:active={slugFilter === 'all'} type="button" title={$t('admin.filterAllTitle')} on:click={() => (slugFilter = 'all')}>{$t('admin.filterAll', { count: playableMatches.length })}</button>
        </div>
      </div>
      <p class="hint">{$t('admin.slugHint')}</p>

      <div class="team-slugs">
        {#if teamSlugMatches.length === 0}
          <p class="empty-state">{slugFilter === 'missing' ? $t('admin.allSlugsPresent') : $t('admin.noMatchesLoaded')}</p>
        {:else}
          {#each teamSlugMatches.slice(0, 40) as m (m.id)}
            <div class="slug-match" class:complete={mappedCode(m.team1.code) && mappedCode(m.team2.code)}>
              <div class="sm-head">
                <span class="lg">{m.league}{m.bestOf ? ` · BO${m.bestOf}` : ''}</span>
                {#if m.hasOdds}<span class="odds-ok">{$t('admin.oddsActive')}</span>{:else}<span class="odds-miss">{$t('admin.oddsNone')}</span>{/if}
              </div>
              {#each [m.team1, m.team2] as team}
                {@const mapped = mappedCode(team.code)}
                {@const key = team.code.trim().toUpperCase()}
                {@const draft = slugEdits[key] ?? mapped ?? ''}
                <div class="slug-team" class:mapped={!!mapped}>
                  <div class="st-id">
                    <span class="st-flag">{mapped ? '✓' : '✗'}</span>
                    <div><strong>{team.code}</strong><small>{team.name}</small></div>
                  </div>
                  <div class="st-edit">
                    <input
                      value={draft}
                      on:input={(e) => setSlug(team.code, e.currentTarget.value)}
                      type="text"
                      placeholder={mapped ? $t('admin.currentPrefix') + mapped : $t('admin.pmPlaceholder')}
                      title={$t('admin.pmCodeFor', { name: team.name, code: team.code })}
                    />
                    <button
                      class="save"
                      type="button"
                      title={$t('admin.saveSlugTitle', { code: team.code })}
                      disabled={savingCode === key || !draft.trim() || draft.trim() === (mapped ?? '')}
                      on:click={() => saveTeamSlug(team.code)}
                    >{savingCode === key ? $t('admin.saving') : mapped ? $t('admin.actionUpdate') : $t('admin.actionCreate')}</button>
                    <button class="test" type="button" title={$t('admin.testSlugFor', { code: team.code })} disabled={slugBusy} on:click={() => testTeamSlug(m, team.code)}><FlaskConical size={14} /></button>
                    {#if mapped}<button class="del" type="button" aria-label={$t('admin.deleteMappingLabel')} title={$t('admin.deleteMappingFor', { code: team.code, mapped })} on:click={() => clearTeamSlug(team.code)}><Trash2 size={14} /></button>{/if}
                  </div>
                </div>
              {/each}
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

  .mapping-context {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    flex-wrap: wrap;
    margin: 0.5rem 0 0.75rem;
    padding: 0.55rem 0.65rem;
    border: 1px solid var(--line);
    border-radius: var(--r-sm);
    background: var(--bg-2);
    font-size: 0.82rem;
  }

  .mapping-context span,
  .mapping-context em {
    color: var(--muted);
    font-style: normal;
  }

  .mapping-context .candidate {
    margin-left: auto;
    color: var(--green);
  }

  .slug-diagnostic {
    display: grid;
    gap: 0.55rem;
    margin: 0.75rem 0;
  }

  .slug-result {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    flex-wrap: wrap;
    padding: 0.55rem 0.65rem;
    border: 1px solid var(--red-soft);
    border-radius: var(--r-sm);
    background: var(--red-soft);
  }

  .slug-result.found {
    border-color: var(--green-soft);
    background: var(--green-soft);
  }

  .slug-result span {
    color: var(--muted);
    font-size: 0.78rem;
    text-transform: uppercase;
  }

  .slug-result a {
    color: var(--green);
  }

  .slug-list {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
    max-height: 7rem;
    overflow-y: auto;
  }

  .slug-list code {
    color: var(--muted);
  }

  .slug-list code.hit {
    color: var(--green);
    border: 1px solid var(--green-soft);
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

  .slug-filter {
    display: flex;
    gap: 0.3rem;
  }

  .slug-filter button {
    min-height: 1.9rem;
    padding: 0 0.7rem;
    border: 1px solid var(--line-2);
    border-radius: 999px;
    color: var(--muted);
    font-size: 0.76rem;
    background: var(--panel-3);
    transition: 120ms ease;
  }

  .slug-filter button.active {
    color: var(--text);
    border-color: var(--green-soft);
    background: var(--green-soft);
  }

  .ok-chip {
    color: var(--green);
  }

  .miss-chip {
    color: var(--amber);
  }

  .team-slugs {
    display: grid;
    gap: 0.5rem;
    margin-top: 0.5rem;
    max-height: 32rem;
    overflow-y: auto;
    scrollbar-width: thin;
  }

  .slug-match {
    display: grid;
    gap: 0.35rem;
    padding: 0.6rem 0.7rem;
    border: 1px solid var(--line);
    border-radius: var(--r-sm);
    background: var(--bg-2);
  }

  .slug-match.complete {
    border-color: var(--green-soft);
  }

  .sm-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    font-size: 0.72rem;
    color: var(--muted);
  }

  .sm-head .lg {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .odds-ok {
    color: var(--green);
  }

  .odds-miss {
    color: var(--amber);
  }

  .slug-team {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.4rem 0.5rem;
    border: 1px solid var(--line);
    border-radius: 8px;
    background: var(--panel);
  }

  .slug-team.mapped {
    border-color: var(--green-soft);
  }

  .st-id {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex: 1 1 9rem;
    min-width: 0;
  }

  .st-flag {
    display: grid;
    place-items: center;
    width: 1.4rem;
    height: 1.4rem;
    flex-shrink: 0;
    border-radius: 6px;
    font-size: 0.8rem;
    color: var(--amber);
    background: hsla(var(--amber-hsl), 0.12);
  }

  .slug-team.mapped .st-flag {
    color: var(--green);
    background: var(--green-soft);
  }

  .st-id div {
    display: grid;
    gap: 0.05rem;
    min-width: 0;
  }

  .st-id strong {
    font-size: 0.88rem;
  }

  .st-id small {
    color: var(--muted);
    font-size: 0.68rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .st-edit {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    flex: 1 1 12rem;
  }

  .st-edit input {
    flex: 1 1 6rem;
    min-width: 0;
    min-height: 2.1rem;
    padding: 0 0.6rem;
    border: 1px solid var(--line);
    border-radius: 7px;
    color: var(--text);
    background: var(--bg-2);
    outline: none;
    text-transform: uppercase;
    font-size: 0.82rem;
  }

  .st-edit input:focus {
    border-color: var(--green);
  }

  .st-edit .save {
    flex-shrink: 0;
    min-height: 2.1rem;
    padding: 0 0.7rem;
    border: 0;
    border-radius: 7px;
    color: #03140c;
    font-weight: 600;
    font-size: 0.78rem;
    background: linear-gradient(180deg, #4ade9f, var(--green));
    transition: 120ms ease;
  }

  .st-edit .save:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .st-edit .test,
  .st-edit .del {
    display: grid;
    place-items: center;
    width: 2.1rem;
    height: 2.1rem;
    flex-shrink: 0;
    border: 1px solid var(--line-2);
    border-radius: 7px;
    color: var(--muted);
    background: transparent;
    transition: 120ms ease;
  }

  .st-edit .test:hover:not(:disabled) {
    color: var(--cyan);
    border-color: var(--cyan);
  }

  .st-edit .del:hover {
    color: var(--red);
    border-color: var(--red);
  }

  @media (max-width: 640px) {
    .status-grid {
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }
    .slug-team {
      flex-wrap: wrap;
    }
  }
</style>
